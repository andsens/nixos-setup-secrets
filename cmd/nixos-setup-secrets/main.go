package main

import (
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
	"github.com/andsens/nixos-setup-secrets/internal/setupsecrets"
)

type CLI struct {
	Verbose bool `help:"Turn on verbose logging"`

	Run struct {
		Auto        bool     `help:"Don't show the TUI, fetch the secrets and store them"`
		ChangedOnly bool     `help:"With --auto, run only the destinations whose command, secrets or outputs changed since they last stored"`
		Group       []string `help:"Only run the destinations of this group, and fetch only what they need. Repeatable"`
	} `cmd:"" default:"withargs" help:"Open the TUI, or with --auto fetch and store in one pass"`

	Cleanup struct {
		Delete bool `help:"Delete them"`
	} `cmd:"" help:"List the outputs setup-secrets wrote that no destination declares any more"`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli, kong.Name("nixos-setup-secrets"), kong.Description("A Nix cli utility for setting up secrets"))
	if cli.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	config, err := setupsecrets.GetConfig()
	if err == nil {
		switch ctx.Command() {
		case "cleanup":
			err = setupsecrets.Cleanup(config, os.Stdout, os.Stderr, cli.Cleanup.Delete)
		default:
			if err = config.SelectGroups(cli.Run.Group); err != nil {
				break
			}
			if cli.Run.Auto {
				err = setupsecrets.SetupAuto(config, cli.Run.ChangedOnly)
			} else {
				err = setupsecrets.SetupManual(config)
			}
		}
	}
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
