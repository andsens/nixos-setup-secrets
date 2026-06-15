package setup_secrets

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Config struct {
	Sources      map[string]*Source `json:"sources"`
	Destinations []*Destination     `json:"destinations"`
}

type Source struct {
	Description string `json:"description"`
	Cmd         string `json:"cmd"`
	Value       *string
}

type Destination struct {
	LogPrefix string   `json:"logPrefix"`
	Requires  []string `json:"requires"`
	Wants     []string `json:"wants"`
	Cmd       string   `json:"cmd"`
}

func GetConfig() (*Config, error) {
	rawConfig := os.Getenv("NIXOS_SETUP_SECRETS_CONFIG")
	if rawConfig == "" {
		return nil, fmt.Errorf("$NIXOS_SETUP_SECRETS_CONFIG is not set or empty")
	}
	var config Config
	if err := json.Unmarshal([]byte(rawConfig), &config); err != nil {
		return nil, fmt.Errorf("Unable to parse $NIXOS_SETUP_SECRETS_CONFIG: %w", err)
	}
	return &config, nil
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
	for name, src := range config.Sources {
		if src.Cmd == "" {
			fmt.Fprintf(log, "%s: Has no fetch command, skipping\n", name)
			continue
		}
		if _, ok := fetchSources[name]; !ok {
			fmt.Fprintf(log, "%s: Not required or needed by any destination, omitting\n", name)
			delete(config.Sources, name)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/usr/bin/env", "bash", "-c", src.Cmd)
		cmd.WaitDelay = 1 * time.Second
		stderr, err := cmd.StderrPipe()
		if err != nil {
			log.Write([]byte(fmt.Errorf("%s: IO error occurred: %w\n", name, err).Error()))
		} else {
			scanner := bufio.NewScanner(stderr)
			go func() {
				for scanner.Scan() {
					fmt.Fprintf(log, "%s: %s\n", name, scanner.Text())
				}
				if err = scanner.Err(); err != nil {
					fmt.Fprintf(log, "%s: %s\n", name, err)
				}
			}()
		}
		fmt.Fprintf(log, "%s: Fetching\n", name)
		var val []byte
		if val, err = cmd.Output(); err != nil {
			fmt.Fprintf(log, "%s\n", fmt.Errorf("%s: Fetching secret failed: %w\n", name, err).Error())
			continue
		}
		value := strings.TrimSpace(string(val))
		src.Value = &value
	}
}

func (config *Config) store(log io.Writer) {
	for _, dest := range config.Destinations {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-c", dest.Cmd)
		cmd.WaitDelay = 1 * time.Second
		cmd.Env = os.Environ()
		skip := false
		for _, srcName := range dest.Requires {
			src, srcFound := config.Sources[srcName]
			if !srcFound {
				fmt.Fprintf(log, "%s: Undefined required secret \"%s\", skipping\n", dest.LogPrefix, srcName)
				skip = true
				continue
			}
			if src.Value == nil || *src.Value == "" {
				fmt.Fprintf(log, "%s: No value for required secret \"%s\", skipping\n", dest.LogPrefix, srcName)
				skip = true
				continue
			}
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", srcName, *src.Value))
		}
		if skip {
			continue
		}
		for _, srcName := range dest.Wants {
			src, srcFound := config.Sources[srcName]
			if !srcFound {
				fmt.Fprintf(log, "%s: Undefined required secret \"%s\", omitting\n", dest.LogPrefix, srcName)
				continue
			}
			if src.Value == nil || *src.Value == "" {
				fmt.Fprintf(log, "%s: No value for required secret \"%s\", omitting\n", dest.LogPrefix, srcName)
				continue
			}
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", srcName, *src.Value))
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			log.Write([]byte(fmt.Errorf("%s: IO error occurred: %w\n", dest.LogPrefix, err).Error()))
		} else {
			scanner := bufio.NewScanner(stderr)
			go func() {
				for scanner.Scan() {
					fmt.Fprintf(log, "%s: %s\n", dest.LogPrefix, scanner.Text())
				}
				if err = scanner.Err(); err != nil {
					fmt.Fprintf(log, "%s: %s\n", dest.LogPrefix, err)
				}
			}()
		}
		fmt.Fprintf(log, "%s: Storing\n", dest.LogPrefix)
		if _, err = cmd.Output(); err != nil {
			fmt.Fprintf(log, "%s\n", fmt.Errorf("%s: Storing secret failed: %w\n", dest.LogPrefix, err).Error())
		}
	}
}
