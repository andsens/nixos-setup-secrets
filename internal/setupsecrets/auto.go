package setupsecrets

import (
	"fmt"
	"os"
	"strings"
)

// SetupAuto fetches and stores in one pass. It fails when a fetch command failed, or a destination failed or was
// skipped for a secret that has a command; a destination waiting for a secret someone has to type in doesn't count
func SetupAuto(config *Config) error {
	fetchFailed := config.fetch(os.Stderr)
	storeFailed := config.store(os.Stderr)
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
