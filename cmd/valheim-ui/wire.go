package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/players"
	"github.com/jonasthim/valheim-server-ui/internal/steam"
)

// wireServices constructs feature services and attaches them to deps. This is
// the single place where concrete types meet; each work package contributes a
// wire_<name>.go with its own constructor function.
func wireServices(ctx context.Context, deps *api.Deps) error {
	// WP-01: auth, users, settings, audit.
	if err := wireAuth(ctx, deps); err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	// WP-04: job runner + SteamCMD client.
	runner, steamClient, err := wireJobs(ctx, deps)
	if err != nil {
		return fmt.Errorf("jobs: %w", err)
	}

	// WP-02: instance service (needs supervisor, bus, db).
	if err := wireInstances(ctx, deps); err != nil {
		return fmt.Errorf("instances: %w", err)
	}
	inst, ok := deps.Instances.(*instance.Service)
	if !ok {
		return fmt.Errorf("instances: unexpected service type %T", deps.Instances)
	}

	// WP-03: log tailing, A2S, players; enriches instance status.
	listRefs := func(ctx context.Context) ([]players.InstanceRef, error) {
		list, err := inst.List(ctx)
		if err != nil {
			return nil, err
		}
		refs := make([]players.InstanceRef, 0, len(list))
		for _, in := range list {
			st := in.Status.State
			refs = append(refs, players.InstanceRef{
				ID:        in.ID,
				Paths:     in.Paths,
				QueryPort: in.Config.Port + 1,
				Running:   st == domain.StateRunning || st == domain.StateStarting,
			})
		}
		return refs, nil
	}
	if err := wirePlayers(ctx, deps, listRefs, inst.Paths, inst.Exists, inst.RegisterEnricher); err != nil {
		return fmt.Errorf("players: %w", err)
	}

	// WP-04: periodic Steam build-id check against every installed instance.
	interval := func() time.Duration {
		s, err := deps.Settings.Get(ctx)
		if err != nil {
			return time.Hour
		}
		return time.Duration(s.Updates.CheckIntervalMinutes) * time.Minute
	}
	listInstalled := func(ctx context.Context) ([]steam.InstalledRef, error) {
		list, err := inst.List(ctx)
		if err != nil {
			return nil, err
		}
		var refs []steam.InstalledRef
		for _, in := range list {
			if in.Status.State != domain.StateNotInstalled {
				refs = append(refs, steam.InstalledRef{ID: in.ID, InstallDir: in.Paths.Server})
			}
		}
		return refs, nil
	}
	store := func(ctx context.Context, id, installed, latest string, at time.Time) error {
		if err := inst.SetBuildIDs(ctx, id, installed, latest, at); err != nil {
			return err
		}
		return inst.PublishStatus(ctx, id)
	}
	checker := steam.NewUpdateChecker(steamClient, interval, listInstalled, store, deps.Bus)
	go checker.Run(ctx)
	deps.Steam = steam.NewService(steamClient, checker)

	// WP-05..08 attach here in wave 2 (install/update jobs, backups, scheduler, mods).
	_ = runner
	return nil
}
