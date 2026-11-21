package setupsecrets

import (
	"fmt"
	"io"
	"slices"
)

// planRun decides which destinations have to run: those never stored, those whose command or secrets changed since,
// and those with a declared output that is gone. A secret changed when its command or token did; one without a same
// command always counts as changed. It returns the reason for each destination that runs
func (config *Config) planRun(log io.Writer, state *State) map[*Destination]string {
	var names []string
	for _, dest := range config.Destinations {
		for _, name := range dest.secrets() {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	tokens := config.sameTokens(log, names)
	changed := map[string]string{}
	keys := map[string]string{}
	for _, name := range names {
		src := config.Sources[name]
		keys[name] = sourceKey(src, tokens[name])
		switch {
		case src == nil || src.Cmd == "":
		case src.Same == "":
			changed[name] = "has no same command"
		case tokens[name] == "" && src.Same != "":
			changed[name] = "same command printed nothing or failed"
		case state.Sources[name] != SourceState{Cmd: src.Cmd, Same: tokens[name]}:
			changed[name] = "changed"
		}
	}
	reasons := map[*Destination]string{}
	for _, dest := range config.Destinations {
		last, stored := state.Destinations[dest.LogPrefix]
		reason := ""
		for _, name := range dest.secrets() {
			if why, ok := changed[name]; ok && reason == "" {
				reason = fmt.Sprintf("%s %s", name, why)
			}
		}
		switch {
		case !stored:
			reason = "never stored"
		case reason != "":
		case last.Key != destKey(dest, keys):
			reason = "its command changed"
		default:
			for _, output := range dest.Outputs {
				exists, err := config.outputExists(log, output)
				if err != nil {
					reason = fmt.Sprintf("can't check %s: %s", output, err)
					break
				}
				if !exists {
					reason = output + " is gone"
					break
				}
			}
		}
		if reason != "" {
			reasons[dest] = reason
		}
	}
	return reasons
}
