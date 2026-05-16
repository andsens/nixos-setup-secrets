package main

import (
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
	setupSecrets "github.com/andsens/nixos-setup-secrets/setup_secrets"
)

type Params struct {
	Auto    bool `help:"Don't show edit dialog, fetch the secrets and immediately store them"`
	Verbose bool `help:"Turn on verbose logging"`
}

var params Params

func main() {
	kong.Parse(&params, kong.Name("nixos-setup-secrets"), kong.Description("A Nix cli utility for setting up secrets "))
	slog.SetDefault(slog.Default())
	if params.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	config, err := setupSecrets.GetConfig()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	if params.Auto {
		err = setupSecrets.SetupAuto(config)
	} else {
		err = setupSecrets.SetupManual(config)
	}
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
