package setupsecrets

import (
	"os"
)

func SetupAuto(config *Config) error {
	config.fetch(os.Stderr)
	config.store(os.Stderr)
	return nil
}
