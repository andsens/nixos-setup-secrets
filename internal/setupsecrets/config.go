package setupsecrets

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"time"
)

type Config struct {
	Sources      map[string]*Source `json:"sources"`
	Destinations []*Destination     `json:"destinations"`
}

// Attempts holds how long a command may run and how often it is retried after a failure
type Attempts struct {
	Timeout string `json:"timeout"`
	Retries int    `json:"retries"`
	timeout time.Duration
}

type Source struct {
	Description string `json:"description"`
	Cmd         string `json:"cmd"`
	Attempts
	Value *string
}

type Destination struct {
	LogPrefix string   `json:"logPrefix"`
	Group     string   `json:"group"`
	Requires  []string `json:"requires"`
	Wants     []string `json:"wants"`
	Cmd       string   `json:"cmd"`
	Attempts
}

const defaultTimeout = 30 * time.Second

func (a *Attempts) parse() error {
	a.timeout = defaultTimeout
	if a.Timeout != "" {
		timeout, err := time.ParseDuration(a.Timeout)
		if err != nil {
			return err
		}
		a.timeout = timeout
	}
	if a.Retries < 0 {
		return fmt.Errorf("retries is negative: %d", a.Retries)
	}
	return nil
}

func GetConfig() (*Config, error) {
	rawConfig := os.Getenv("NIXOS_SETUP_SECRETS_CONFIG")
	if rawConfig == "" {
		return nil, fmt.Errorf("$NIXOS_SETUP_SECRETS_CONFIG is not set or empty")
	}
	return parseConfig([]byte(rawConfig))
}

func parseConfig(rawConfig []byte) (*Config, error) {
	var config Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("Unable to parse $NIXOS_SETUP_SECRETS_CONFIG: %w", err)
	}
	for name, src := range config.Sources {
		if err := src.parse(); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	for _, dest := range config.Destinations {
		if err := dest.parse(); err != nil {
			return nil, fmt.Errorf("%s: %w", dest.LogPrefix, err)
		}
	}
	return &config, nil
}

// SelectGroups keeps only the destinations of the given groups, all of them when none are given. Sources no kept
// destination requires or wants are left out of the fetch.
func (config *Config) SelectGroups(groups []string) error {
	if len(groups) == 0 {
		return nil
	}
	known := map[string]struct{}{}
	for _, dest := range config.Destinations {
		known[dest.Group] = struct{}{}
	}
	for _, group := range groups {
		if _, ok := known[group]; !ok {
			return fmt.Errorf("No destination is in the group %q, known groups: %s", group,
				strings.Join(slices.Sorted(maps.Keys(known)), ", "))
		}
	}
	config.Destinations = slices.DeleteFunc(config.Destinations, func(dest *Destination) bool {
		return !slices.Contains(groups, dest.Group)
	})
	return nil
}

func (config *Config) fetch(log io.Writer) {
	fetchSources := map[string]struct{}{}
	for _, dest := range config.Destinations {
		for _, srcName := range dest.Wants {
			fetchSources[srcName] = struct{}{}
		}
		for _, srcName := range dest.Requires {
			fetchSources[srcName] = struct{}{}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(config.Sources)) {
		src := config.Sources[name]
		if _, ok := fetchSources[name]; !ok {
			fmt.Fprintf(log, "%s: Not required or needed by any destination, omitting\n", name)
			delete(config.Sources, name)
			continue
		}
		if src.Cmd == "" {
			fmt.Fprintf(log, "%s: Has no fetch command, skipping\n", name)
			continue
		}
		fmt.Fprintf(log, "%s: Fetching\n", name)
		val, err := run(log, name, src.Cmd, nil, src.Attempts)
		if err != nil {
			fmt.Fprintf(log, "%s: Fetching secret failed: %s\n", name, err)
			continue
		}
		value := strings.TrimSpace(string(val))
		src.Value = &value
	}
}

func (config *Config) store(log io.Writer) {
	for _, dest := range config.Destinations {
		env, ok := config.destinationEnv(log, dest)
		if !ok {
			continue
		}
		fmt.Fprintf(log, "%s: Storing\n", dest.LogPrefix)
		if _, err := run(log, dest.LogPrefix, dest.Cmd, env, dest.Attempts); err != nil {
			fmt.Fprintf(log, "%s: Storing secret failed: %s\n", dest.LogPrefix, err)
		}
	}
}

// destinationEnv returns the variables of the secrets a destination requires and wants, or false when a required
// one has no value
func (config *Config) destinationEnv(log io.Writer, dest *Destination) ([]string, bool) {
	var env []string
	ok := true
	for _, srcName := range dest.Requires {
		src, srcFound := config.Sources[srcName]
		if !srcFound {
			fmt.Fprintf(log, "%s: Undefined required secret \"%s\", skipping\n", dest.LogPrefix, srcName)
			ok = false
			continue
		}
		if src.Value == nil || *src.Value == "" {
			fmt.Fprintf(log, "%s: No value for required secret \"%s\", skipping\n", dest.LogPrefix, srcName)
			ok = false
			continue
		}
		env = append(env, fmt.Sprintf("%s=%s", srcName, *src.Value))
	}
	if !ok {
		return nil, false
	}
	for _, srcName := range dest.Wants {
		src, srcFound := config.Sources[srcName]
		if !srcFound {
			fmt.Fprintf(log, "%s: Undefined wanted secret \"%s\", omitting\n", dest.LogPrefix, srcName)
			continue
		}
		if src.Value == nil || *src.Value == "" {
			fmt.Fprintf(log, "%s: No value for wanted secret \"%s\", omitting\n", dest.LogPrefix, srcName)
			continue
		}
		env = append(env, fmt.Sprintf("%s=%s", srcName, *src.Value))
	}
	return env, true
}
