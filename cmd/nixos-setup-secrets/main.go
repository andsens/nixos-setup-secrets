package main

import (
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
	"github.com/andsens/nixos-setup-secrets/internal/setupsecrets"
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
	config, err := setupsecrets.GetConfig()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	if params.Auto {
		err = setupsecrets.SetupAuto(config)
	} else {
		err = setupsecrets.SetupManual(config)
	}
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
