package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/selfupdate"
)

// selfUpgradeUsage documents the break-glass path: if the web UI itself is
// broken (a bad release, a crash loop), an admin with shell access can still
// check for, install, or undo a manager release without going through the
// HTTP API at all.
const selfUpgradeUsage = `usage: valheim-ui self-upgrade <--check|--apply|--rollback> [flags]
  --check            query GitHub for the latest release and report it
  --apply            download, verify and install a newer release
      --version vX.Y.Z   install this tag instead of the latest release
  --rollback         restore the binary kept at <path>.prev by a previous --apply
  --config PATH      config file (default /etc/valheim-ui/config.yaml)

This is the break-glass path for a manager that cannot serve its own web UI;
normally upgrades are triggered from Settings or the scheduled checker
(docs/ARCHITECTURE.md §16), which restart the systemd unit automatically.
After --apply or --rollback here, restart it yourself:
  systemctl restart valheim-ui
`

func runSelfUpgrade(args []string) error {
	fs := flag.NewFlagSet("self-upgrade", flag.ContinueOnError)
	check := fs.Bool("check", false, "check for a newer release")
	apply := fs.Bool("apply", false, "download and install a newer release")
	rollback := fs.Bool("rollback", false, "restore the previous binary")
	targetVersion := fs.String("version", "", "release tag to install (--apply only; default: latest)")
	cfgPath := fs.String("config", envOr("VALHEIM_UI_CONFIG", config.DefaultPath), "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	chosen := 0
	for _, b := range []bool{*check, *apply, *rollback} {
		if b {
			chosen++
		}
	}
	if chosen != 1 {
		fmt.Fprint(os.Stderr, selfUpgradeUsage)
		return errors.New("self-upgrade: exactly one of --check, --apply, --rollback is required")
	}

	// self-upgrade must keep working even when the config file is missing or
	// the data directory is unwritable elsewhere: config.Load falls back to
	// defaults for a missing file, so this only fails on a genuinely broken
	// config.yaml.
	if _, err := config.Load(*cfgPath); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("self-upgrade: resolve executable: %w", err)
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("self-upgrade: resolve real binary path (is %s a broken symlink?): %w", exe, err)
	}

	client := selfupdate.NewClient(domain.GitHubRepo, version)

	switch {
	case *check:
		return selfUpgradeCheck(client)
	case *apply:
		return selfUpgradeApply(client, real, *targetVersion)
	default: // --rollback
		return selfUpgradeRollback(real)
	}
}

func selfUpgradeCheck(client *selfupdate.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rel, err := client.Latest(ctx)
	if err != nil {
		return fmt.Errorf("self-upgrade: %w", err)
	}
	fmt.Printf("current version: %s\n", version)
	if rel == nil {
		fmt.Println("no releases are published yet")
		return nil
	}
	fmt.Printf("latest release:  %s\n", rel.Tag)
	if rel.HTMLURL != "" {
		fmt.Printf("release notes:   %s\n", rel.HTMLURL)
	}
	if selfupdate.Compare(rel.Tag, version) > 0 {
		fmt.Println("a newer version is available (valheim-ui self-upgrade --apply)")
	} else {
		fmt.Println("already up to date")
	}
	return nil
}

func selfUpgradeApply(client *selfupdate.Client, exePath, tag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	rel, err := resolveRelease(ctx, client, tag)
	if err != nil {
		return fmt.Errorf("self-upgrade: %w", err)
	}

	fmt.Printf("installing %s (currently running %s)\n", rel.Tag, version)
	upgrader := selfupdate.NewUpgrader(exePath, version, http.DefaultClient)
	prevPath, err := upgrader.Apply(ctx, rel, os.Stdout)
	if err != nil {
		return fmt.Errorf("self-upgrade: %w", err)
	}
	fmt.Printf("done; previous binary kept at %s\n", prevPath)
	fmt.Println("restart the service to run it: systemctl restart valheim-ui")
	return nil
}

func selfUpgradeRollback(exePath string) error {
	// version isn't known to be accurate here (the running process may
	// already be the bad build), but Rollback doesn't need it.
	upgrader := selfupdate.NewUpgrader(exePath, version, http.DefaultClient)
	if err := upgrader.Rollback(); err != nil {
		return fmt.Errorf("self-upgrade: %w", err)
	}
	fmt.Println("rolled back to the previous binary")
	fmt.Println("restart the service to run it: systemctl restart valheim-ui")
	return nil
}

func resolveRelease(ctx context.Context, client *selfupdate.Client, tag string) (*selfupdate.Release, error) {
	if tag == "" {
		rel, err := client.Latest(ctx)
		if err != nil {
			return nil, err
		}
		if rel == nil {
			return nil, errors.New("no releases are published yet")
		}
		return rel, nil
	}
	rel, err := client.ByTag(ctx, tag)
	if err != nil {
		return nil, err
	}
	if rel == nil {
		return nil, fmt.Errorf("release %s not found", tag)
	}
	return rel, nil
}
