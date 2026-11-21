package setupsecrets

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Sources      map[string]*Source     `json:"sources"`
	Destinations []*Destination         `json:"destinations"`
	OutputKinds  map[string]*OutputKind `json:"outputKinds"`
	// Every destination, before SelectGroups narrows Destinations: what cleanup compares the state with
	all []*Destination
	// Where the state is kept, empty for none
	statePath string
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
	// Prints a cheap token that changes when the value does, without fetching it
	Same string `json:"same"`
	Attempts
	Value *string
	// Whether the last run of Cmd failed, as opposed to succeeding without printing a value
	failed bool
}

type Destination struct {
	LogPrefix string   `json:"logPrefix"`
	Group     string   `json:"group"`
	Requires  []string `json:"requires"`
	Wants     []string `json:"wants"`
	Cmd       string   `json:"cmd"`
	// What the command writes, as kind:id
	Outputs []string `json:"outputs"`
	Attempts
}

// OutputKind holds the commands that check and delete an output of a kind, given its id as $OUTPUT. Exists exits 0
// when the output is there and 1 when it isn't; anything else is an error
type OutputKind struct {
	Exists string `json:"exists"`
	Delete string `json:"delete"`
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
	config, err := parseConfig([]byte(rawConfig))
	if err != nil {
		return nil, err
	}
	config.statePath = os.Getenv("NIXOS_SETUP_SECRETS_STATE")
	return config, nil
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
		for _, output := range dest.Outputs {
			if _, _, ok := strings.Cut(output, ":"); !ok {
				return nil, fmt.Errorf("%s: output %q isn't kind:id", dest.LogPrefix, output)
			}
		}
	}
	config.all = slices.Clone(config.Destinations)
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

// fetch runs the command of the source and keeps what it prints as the value
func (src *Source) fetch(log io.Writer, name string) error {
	value, err := src.fetchValue(log, name)
	src.failed = err != nil
	if err == nil {
		src.Value = &value
	}
	return err
}

// fetchValue runs the command of the source and returns what it prints, leaving the source as it is
func (src *Source) fetchValue(log io.Writer, name string) (string, error) {
	fmt.Fprintf(log, "%s: Fetching\n", name)
	val, err := run(log, name, src.Cmd, nil, src.Attempts)
	if err != nil {
		fmt.Fprintf(log, "%s: Fetching secret failed: %s\n", name, err)
		return "", err
	}
	value := strings.TrimSpace(string(val))
	if value == "" {
		fmt.Fprintf(log, "%s: No value yet\n", name)
	}
	return value, nil
}

// Needs says whether the destination requires or wants the secret
func (dest *Destination) Needs(name string) bool {
	return slices.Contains(dest.Requires, name) || slices.Contains(dest.Wants, name)
}

// secrets lists what the destination requires and wants
func (dest *Destination) secrets() []string {
	return append(slices.Clone(dest.Requires), dest.Wants...)
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
	return runDestination(log, dest, env)
}

// runDestination runs the command of the destination with the variables destinationEnv returned
func runDestination(log io.Writer, dest *Destination, env []string) storeResult {
	fmt.Fprintf(log, "%s: Storing\n", dest.LogPrefix)
	if _, err := run(log, dest.LogPrefix, dest.Cmd, env, dest.Attempts); err != nil {
		fmt.Fprintf(log, "%s: Storing secret failed: %s\n", dest.LogPrefix, err)
		return storeFailed
	}
	return storeDone
}

// fetchAndStore fetches what the destinations need a few at a time and runs each destination as soon as every secret
// it requires or wants is fetched, so a source that waits for a workload doesn't hold up the destinations that
// workload waits for. It returns the sources that failed, the destinations that failed or were skipped for a secret
// whose command failed, and the destinations it stored
func (config *Config) fetchAndStore(log io.Writer, dests []*Destination, workers int) (failedSources, failedDests []string, stored []*Destination) {
	pending := map[string]bool{}
	for _, dest := range dests {
		for _, name := range dest.secrets() {
			if src, ok := config.Sources[name]; ok && src.Cmd != "" {
				pending[name] = true
			}
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, workers)
	started := make([]bool, len(dests))
	// startReady starts the destinations whose secrets are all fetched; mu is held
	startReady := func() {
		for i, dest := range dests {
			if started[i] || slices.ContainsFunc(dest.secrets(), func(name string) bool { return pending[name] }) {
				continue
			}
			started[i] = true
			wg.Go(func() {
				slots <- struct{}{}
				result := config.storeOne(log, dest)
				<-slots
				mu.Lock()
				defer mu.Unlock()
				switch result {
				case storeDone:
					stored = append(stored, dest)
				case storeFailed, storeSkippedFault:
					failedDests = append(failedDests, dest.LogPrefix)
				}
			})
		}
	}
	mu.Lock()
	startReady()
	for _, name := range slices.Sorted(maps.Keys(pending)) {
		src := config.Sources[name]
		wg.Go(func() {
			slots <- struct{}{}
			err := src.fetch(log, name)
			<-slots
			mu.Lock()
			defer mu.Unlock()
			delete(pending, name)
			if err != nil {
				failedSources = append(failedSources, name)
			}
			startReady()
		})
	}
	mu.Unlock()
	wg.Wait()
	slices.Sort(failedSources)
	slices.Sort(failedDests)
	return failedSources, failedDests, stored
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
			// A command that succeeded without a value has nothing yet, like a secret without a command
			fault = fault || src.failed
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
