package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/events"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
	"github.com/jonasthim/valheim-server-ui/web"
)

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	cfgPath := fs.String("config", envOr("VALHEIM_UI_CONFIG", config.DefaultPath), "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	for _, dir := range []string{cfg.DataDir, cfg.InstancesDir(), cfg.JobsDir(), cfg.CacheDir()} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sqldb, err := db.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer sqldb.Close()

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	sup, err := supervisor.New(cfg.Supervisor, supervisor.Options{
		SelfPath: self, InstancesDir: cfg.InstancesDir(), UnitctlPath: cfg.UnitctlPath, Log: log,
	})
	if err != nil {
		return err
	}

	deps := &api.Deps{
		Cfg: cfg, Log: log, DB: sqldb, Bus: events.NewBus(), Supervisor: sup,
		Version: version, Commit: commit, StartedAt: time.Now(),
	}

	// Wave-1+ packages are constructed and attached here (see WORKPLAN.md).
	if err := wireServices(ctx, deps); err != nil {
		return err
	}
	if deps.Auth == nil && !cfg.DevNoAuth {
		return errors.New("authentication is not wired; refusing to start (set dev_no_auth for local development)")
	}
	if cfg.DevNoAuth {
		log.Warn("DEV_NO_AUTH is enabled: every request runs as admin")
	}

	dist, err := web.Dist()
	if err != nil {
		return fmt.Errorf("embedded frontend: %w", err)
	}
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.NewRouter(deps, api.SPAHandler(dist)),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Listen, "version", version, "supervisor", sup.Kind(), "data_dir", cfg.DataDir)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lv}))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
