package setupsecrets

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// outputCommand finds the command of the kind of output and its environment
func (config *Config) outputCommand(output string, command func(*OutputKind) string) (string, []string, error) {
	kind, id, _ := strings.Cut(output, ":")
	k, ok := config.OutputKinds[kind]
	if !ok {
		return "", nil, fmt.Errorf("no output kind %q", kind)
	}
	script := command(k)
	if script == "" {
		return "", nil, fmt.Errorf("output kind %q has no command for this", kind)
	}
	return script, []string{"OUTPUT=" + id}, nil
}

// outputExists checks an output with the exists command of its kind: exit 0 is there, exit 1 is gone, anything
// else an error
func (config *Config) outputExists(log io.Writer, output string) (bool, error) {
	script, env, err := config.outputCommand(output, func(k *OutputKind) string { return k.Exists })
	if err != nil {
		return false, err
	}
	_, err = run(log, output, script, env, Attempts{timeout: defaultTimeout})
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exitErr) && exitErr.ExitCode() == 1:
		return false, nil
	}
	return false, err
}

// deleteOutput deletes an output with the delete command of its kind
func (config *Config) deleteOutput(log io.Writer, output string) error {
	script, env, err := config.outputCommand(output, func(k *OutputKind) string { return k.Delete })
	if err != nil {
		return err
	}
	_, err = run(log, output, script, env, Attempts{timeout: defaultTimeout})
	return err
}
