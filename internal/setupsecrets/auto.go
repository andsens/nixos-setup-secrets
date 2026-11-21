package setupsecrets

import (
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
)

// SetupAuto fetches and stores in one pass, only what changed with changedOnly. It fails when a fetch command
// failed, or a destination failed or was skipped for a secret that has a command; a destination waiting for a secret
// someone has to type in doesn't count
func SetupAuto(config *Config, changedOnly bool) error {
	return setupAuto(config, os.Stderr, changedOnly)
}

func setupAuto(config *Config, log io.Writer, changedOnly bool) error {
	dests := config.Destinations
	if changedOnly {
		state, err := readState(config.statePath)
		if err != nil {
			return err
		}
		reasons := config.planRun(log, state)
		dests = slices.DeleteFunc(slices.Clone(dests), func(dest *Destination) bool {
			_, runs := reasons[dest]
			if !runs {
				fmt.Fprintf(log, "%s: Unchanged\n", dest.LogPrefix)
			}
			return !runs
		})
		for _, dest := range dests {
			fmt.Fprintf(log, "%s: Runs, %s\n", dest.LogPrefix, reasons[dest])
		}
	}
	needed := map[string]bool{}
	for _, dest := range dests {
		for _, name := range dest.secrets() {
			needed[name] = true
		}
	}
	for _, name := range slices.Sorted(maps.Keys(config.Sources)) {
		if !needed[name] {
			fmt.Fprintf(log, "%s: Not needed by any destination that runs, omitting\n", name)
		} else if config.Sources[name].Cmd == "" {
			fmt.Fprintf(log, "%s: Has no fetch command, skipping\n", name)
		}
	}
	fetchFailed, storeFailed, stored := config.fetchAndStore(log, dests, 8)
	if err := config.record(log, stored); err != nil {
		fmt.Fprintf(log, "Recording the state failed: %s\n", err)
	}
	var problems []string
	if len(fetchFailed) > 0 {
		problems = append(problems, "fetching "+strings.Join(fetchFailed, ", "))
	}
	if len(storeFailed) > 0 {
		problems = append(problems, "storing "+strings.Join(storeFailed, ", "))
	}
	if len(problems) > 0 {
		return fmt.Errorf("Failed %s", strings.Join(problems, "; "))
	}
	return nil
}
