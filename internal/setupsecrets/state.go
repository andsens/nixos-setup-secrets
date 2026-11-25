package setupsecrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

// State is what setup-secrets remembers between runs: names, command strings and tokens, never values
type State struct {
	Sources      map[string]SourceState `json:"sources"`
	Destinations map[string]DestState   `json:"destinations"`
	// Every output a destination wrote, by kind:id, until cleanup deletes it
	Outputs map[string]OutputState `json:"outputs"`
}

type SourceState struct {
	Cmd  string `json:"cmd"`
	Same string `json:"same"`
}

type DestState struct {
	// The command of the destination and the keys of its secrets when it last stored
	Key     string   `json:"key"`
	Outputs []string `json:"outputs"`
}

type OutputState struct {
	WrittenBy string    `json:"writtenBy"`
	At        time.Time `json:"at"`
}

func newState() *State {
	return &State{Sources: map[string]SourceState{}, Destinations: map[string]DestState{}, Outputs: map[string]OutputState{}}
}

// readState reads the state, empty when the file doesn't exist or no path is set
func readState(path string) (*State, error) {
	state := newState()
	if path == "" {
		return state, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, state); err != nil {
		return nil, fmt.Errorf("Unable to parse %s: %w", path, err)
	}
	if state.Sources == nil {
		state.Sources = map[string]SourceState{}
	}
	if state.Destinations == nil {
		state.Destinations = map[string]DestState{}
	}
	if state.Outputs == nil {
		state.Outputs = map[string]OutputState{}
	}
	return state, nil
}

// updateState changes the state under an exclusive lock, as the units of the groups run at the same time
func updateState(path string, change func(*State)) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	state, err := readState(path)
	if err != nil {
		return err
	}
	change(state)
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// sourceKey is what makes a source the same as when its destination last stored: its command and its token. A
// source without a command only changes when someone types a new value, which the TUI records itself
func sourceKey(src *Source, token string) string {
	if src == nil || src.Cmd == "" {
		return "manual"
	}
	return src.Cmd + "\x1f" + token
}

// destKey is the command of the destination with the keys of its secrets
func destKey(dest *Destination, srcKeys map[string]string) string {
	names := slices.Sorted(slices.Values(dest.secrets()))
	var sb strings.Builder
	sb.WriteString(dest.Cmd)
	for _, name := range names {
		fmt.Fprintf(&sb, "\n%s=%s", name, srcKeys[name])
	}
	return sb.String()
}

// sameTokens runs the same commands of the sources and returns the tokens of those that exited 0, an empty one
// included. For the others it returns why there is no token: the command can't tell yet (it exited with
// exitTempFail, which isn't retried), or it failed. A source without a same command is in neither
func (config *Config) sameTokens(log io.Writer, names []string) (tokens, unknown map[string]string) {
	tokens, unknown = map[string]string{}, map[string]string{}
	for _, name := range names {
		src := config.Sources[name]
		if src == nil || src.Cmd == "" || src.Same == "" {
			continue
		}
		cantTell := func(err error) bool { return exitedWith(err, exitTempFail) }
		out, err := runUntil(log, name, src.Same, nil, src.Attempts, cantTell)
		switch {
		case err == nil:
			tokens[name] = strings.TrimSpace(string(out))
		case cantTell(err):
			fmt.Fprintf(log, "%s: Same command can't tell yet\n", name)
			unknown[name] = "same command can't tell yet"
		default:
			fmt.Fprintf(log, "%s: Same command failed: %s\n", name, err)
			unknown[name] = "same command failed"
		}
	}
	return tokens, unknown
}

// record keeps what the stored destinations wrote and the tokens of their secrets, taken after storing, as a
// destination that writes what its source reads changes the token
func (config *Config) record(log io.Writer, stored []*Destination) error {
	if config.statePath == "" || len(stored) == 0 {
		return nil
	}
	var names []string
	for _, dest := range stored {
		for _, name := range dest.secrets() {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	tokens, unknown := config.sameTokens(log, names)
	now := time.Now().UTC().Truncate(time.Second)
	return updateState(config.statePath, func(state *State) {
		keys := map[string]string{}
		for _, name := range names {
			src := config.Sources[name]
			keys[name] = sourceKey(src, tokens[name])
			switch {
			case src == nil || src.Cmd == "":
			case unknown[name] != "":
				// No token to remember, so the next run counts it as changed
				delete(state.Sources, name)
			default:
				state.Sources[name] = SourceState{Cmd: src.Cmd, Same: tokens[name]}
			}
		}
		for _, dest := range stored {
			state.Destinations[dest.LogPrefix] = DestState{Key: destKey(dest, keys), Outputs: dest.Outputs}
			for _, output := range dest.Outputs {
				state.Outputs[output] = OutputState{WrittenBy: dest.LogPrefix, At: now}
			}
		}
	})
}

// sortedOutputs lists the outputs of the state in order
func (state *State) sortedOutputs() []string {
	return slices.Sorted(maps.Keys(state.Outputs))
}
