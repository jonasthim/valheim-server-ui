package main

import (
	"errors"
	"flag"

	"github.com/jonasthim/valheim-server-ui/internal/config"
)

// runLaunch is invoked by systemd: ExecStart=/usr/local/bin/valheim-ui launch --instance %i
// WP-02 implements it in internal/launcher and replaces the body below.
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
	_ = cfgPath
	return errors.New("launch: not implemented (WP-02)")
}
