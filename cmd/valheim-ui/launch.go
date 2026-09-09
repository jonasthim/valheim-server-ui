package main

import (
	"context"
	"errors"
	"flag"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/launcher"
)

// runLaunch is invoked by systemd (or the direct supervisor):
// ExecStart=/usr/local/bin/valheim-ui launch --instance %i
//
// On success it never returns: launcher.Run replaces this process image with
// the game server (or the fake server in dev/test).
func runLaunch(args []string) error {
	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	instance := fs.String("instance", "", "instance id")
	cfgPath := fs.String("config", envOr("VALHEIM_UI_CONFIG", config.DefaultPath), "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *instance == "" {
		return errors.New("--instance is required")
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	return launcher.Run(context.Background(), cfg, *instance)
}
