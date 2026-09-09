package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
)

func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	cfgPath := fs.String("config", envOr("VALHEIM_UI_CONFIG", config.DefaultPath), "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	sqldb, err := db.Open(context.Background(), cfg.DBPath())
	if err != nil {
		return err
	}
	defer sqldb.Close()
	fmt.Println("migrations applied:", cfg.DBPath())
	return nil
}
