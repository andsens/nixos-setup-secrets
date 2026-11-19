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
	// Where a destination stores the value that Cmd reads back, which makes an edited value stick
	StoredIn string `json:"storedIn"`
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

// fetch runs the commands of the sources the destinations need and returns the names of those that failed
func (config *Config) fetch(log io.Writer) (failed []string) {
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
		if err := src.fetch(log, name); err != nil {
			failed = append(failed, name)
		}
	}
	return failed
}

// fetch runs the command of the source and keeps what it prints as the value
func (src *Source) fetch(log io.Writer, name string) error {
	fmt.Fprintf(log, "%s: Fetching\n", name)
	val, err := run(log, name, src.Cmd, nil, src.Attempts)
	if err != nil {
		fmt.Fprintf(log, "%s: Fetching secret failed: %s\n", name, err)
		return err
	}
	value := strings.TrimSpace(string(val))
	src.Value = &value
	return nil
}

// Needs says whether the destination requires or wants the secret
func (dest *Destination) Needs(name string) bool {
	return slices.Contains(dest.Requires, name) || slices.Contains(dest.Wants, name)
}

// store runs the destinations and returns those that failed, or were skipped for a required secret that has a
// command but no value. A destination waiting for a secret without a command isn't returned: in an automatic run
// that value can only come from someone typing it in
func (config *Config) store(log io.Writer) (failed []string) {
	for _, dest := range config.Destinations {
		if result := config.storeOne(log, dest); result == storeFailed || result == storeSkippedFault {
			failed = append(failed, dest.LogPrefix)
		}
	}
	return failed
}

type storeResult int

const (
	storeDone storeResult = iota
	storeFailed
	// Skipped for a required secret without a value that only someone typing it in can supply
	storeWaiting
	// Skipped for a required secret that is undefined or whose command didn't deliver
	storeSkippedFault
)

func (config *Config) storeOne(log io.Writer, dest *Destination) storeResult {
	env, ok, fault := config.destinationEnv(log, dest)
	if !ok {
		if fault {
			return storeSkippedFault
		}
		return storeWaiting
	}
	fmt.Fprintf(log, "%s: Storing\n", dest.LogPrefix)
	if _, err := run(log, dest.LogPrefix, dest.Cmd, env, dest.Attempts); err != nil {
		fmt.Fprintf(log, "%s: Storing secret failed: %s\n", dest.LogPrefix, err)
		return storeFailed
	}
	return storeDone
}

// destinationEnv returns the variables of the secrets a destination requires and wants, or false when a required
// one has no value; fault says whether one of those is undefined or has a command that didn't deliver
func (config *Config) destinationEnv(log io.Writer, dest *Destination) (env []string, ok bool, fault bool) {
	ok = true
	for _, srcName := range dest.Requires {
		src, srcFound := config.Sources[srcName]
		if !srcFound {
			fmt.Fprintf(log, "%s: Undefined required secret \"%s\", skipping\n", dest.LogPrefix, srcName)
			ok, fault = false, true
			continue
		}
		if src.Value == nil || *src.Value == "" {
			fmt.Fprintf(log, "%s: No value for required secret \"%s\", skipping\n", dest.LogPrefix, srcName)
			ok = false
			fault = fault || src.Cmd != ""
			continue
		}
		env = append(env, fmt.Sprintf("%s=%s", srcName, *src.Value))
	}
	if !ok {
		return nil, false, fault
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
	return env, true, false
}
