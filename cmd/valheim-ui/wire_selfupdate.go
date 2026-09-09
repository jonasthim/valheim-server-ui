package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
	"github.com/jonasthim/valheim-server-ui/internal/selfupdate"
)

// wireSelfUpdate constructs the GitHub releases client, the periodic
// self-upgrade checker and the upgrade Service (WP-30), attaches
// deps.SelfUpdate, connects the checker's auto-upgrade hook to the service,
// and starts the checker's background loop bound to ctx.
//
// wire.go should call this from wireServices once deps.Settings (WP-01) and
// the job runner (WP-04, the same *jobs.Runner returned by wireJobs) exist,
// passing a playersOnline callback that reports the total number of players
// connected across every instance (e.g. summed from deps.Players / the
// players.Manager via players.PlayersOnline for each instance id), since
// auto-upgrade only ever proceeds when nobody is playing anywhere.
func wireSelfUpdate(ctx context.Context, deps *api.Deps, runner *jobs.Runner, playersOnline func(ctx context.Context) (int, error)) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("wire self-update: resolve executable: %w", err)
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("wire self-update: resolve real binary path: %w", err)
	}

	// VALHEIM_UI_RELEASES_BASE_URL points the release client at a mirror or a
	// test server (development only; production uses api.github.com).
	var copts []selfupdate.ClientOption
	if base := os.Getenv("VALHEIM_UI_RELEASES_BASE_URL"); base != "" {
		copts = append(copts, selfupdate.WithBaseURL(base))
	}
	client := selfupdate.NewClient(domain.GitHubRepo, version, copts...)

	interval := func() time.Duration {
		s, err := deps.Settings.Get(ctx)
		if err != nil {
			return time.Hour
		}
		return time.Duration(s.App.UpdateCheckHours) * time.Hour
	}
	autoUpgrade := func() bool {
		s, err := deps.Settings.Get(ctx)
		if err != nil {
			return false
		}
		return s.App.AutoUpgrade
	}

	checker := selfupdate.New(client, version, real, interval, autoUpgrade, playersOnline, deps.Bus, deps.Log)
	upgrader := selfupdate.NewUpgrader(real, version, http.DefaultClient)
	svc := selfupdate.NewService(client, checker, upgrader, runner, deps.Log)

	// The checker calls back into the service to actually perform an
	// automatic upgrade (enqueue the job); wired here, after both exist,
	// since the service itself needs the checker (for CanSelfUpgrade) to
	// construct.
	checker.SetUpgradeFunc(func(ctx context.Context, ver string) error {
		_, err := svc.EnqueueUpgrade(ctx, ver, "auto-upgrade")
		return err
	})

	deps.SelfUpdate = svc
	go checker.Run(ctx)
	return nil
}
