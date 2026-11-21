package setupsecrets

import (
	"fmt"
	"io"
	"slices"
)

// Orphan is an output a destination wrote that no destination declares any more
type Orphan struct {
	Output string
	OutputState
	// Exists is false when the output turned out to be gone, Err set when checking it failed
	Exists bool
	Err    error
}

// orphans lists the outputs in the state that no destination of the whole configuration declares
func (config *Config) orphans(log io.Writer) ([]Orphan, error) {
	state, err := readState(config.statePath)
	if err != nil {
		return nil, err
	}
	declared := map[string]bool{}
	for _, dest := range config.all {
		for _, output := range dest.Outputs {
			declared[output] = true
		}
	}
	var out []Orphan
	for _, output := range state.sortedOutputs() {
		if declared[output] {
			continue
		}
		orphan := Orphan{Output: output, OutputState: state.Outputs[output]}
		orphan.Exists, orphan.Err = config.outputExists(log, output)
		out = append(out, orphan)
	}
	return out, nil
}

// forget drops outputs from the state
func (config *Config) forget(outputs ...string) error {
	return updateState(config.statePath, func(state *State) {
		for _, output := range outputs {
			delete(state.Outputs, output)
		}
	})
}

// removeOrphan deletes an orphan and drops it from the state
func (config *Config) removeOrphan(log io.Writer, output string) error {
	if err := config.deleteOutput(log, output); err != nil {
		return err
	}
	return config.forget(output)
}

// Cleanup lists the outputs no destination declares any more, deleting them with del. Those already gone are
// dropped from the state either way
func Cleanup(config *Config, out, log io.Writer, del bool) error {
	if config.statePath == "" {
		return fmt.Errorf("No state file: $NIXOS_SETUP_SECRETS_STATE is not set")
	}
	orphans, err := config.orphans(log)
	if err != nil {
		return err
	}
	var gone []string
	var failed []string
	for _, orphan := range orphans {
		at := orphan.At.Local().Format("2006-01-02 15:04")
		switch {
		case orphan.Err != nil:
			fmt.Fprintf(out, "%-50s written by %s, %s: can't check: %s\n", orphan.Output, orphan.WrittenBy, at, orphan.Err)
			failed = append(failed, orphan.Output)
		case !orphan.Exists:
			fmt.Fprintf(out, "%-50s written by %s, %s: already gone, forgotten\n", orphan.Output, orphan.WrittenBy, at)
			gone = append(gone, orphan.Output)
		case del:
			if err := config.removeOrphan(log, orphan.Output); err != nil {
				fmt.Fprintf(out, "%-50s written by %s, %s: deleting failed: %s\n", orphan.Output, orphan.WrittenBy, at, err)
				failed = append(failed, orphan.Output)
				continue
			}
			fmt.Fprintf(out, "%-50s written by %s, %s: deleted\n", orphan.Output, orphan.WrittenBy, at)
		default:
			fmt.Fprintf(out, "%-50s written by %s, %s\n", orphan.Output, orphan.WrittenBy, at)
		}
	}
	if len(orphans) == 0 {
		fmt.Fprintln(out, "Every output setup-secrets wrote is still declared.")
	} else if !del && len(orphans) > len(gone)+len(failed) {
		fmt.Fprintln(out, "\nsetup-secrets cleanup --delete deletes them.")
	}
	if err := config.forget(gone...); err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("Failed for %s", slices.Clone(failed))
	}
	return nil
}
