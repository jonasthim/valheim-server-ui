package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/metrics"
	"github.com/jonasthim/valheim-server-ui/internal/players"
	"github.com/jonasthim/valheim-server-ui/internal/scheduler"
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
	// Host and per-process CPU/memory from /proc; enriches running instances
	// and feeds the dashboard tiles.
	sampler := metrics.New()
	sampler.Prime()
	inst.RegisterEnricher(sampler)
	deps.Metrics = sampler

	playersMgr, err := wirePlayers(ctx, deps, listRefs, inst.Paths, inst.Exists, inst.RegisterEnricher)
	if err != nil {
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

	// WP-06: backups and worlds.
	backups := wireBackups(deps, inst, runner)

	// WP-05: install/update jobs, with a pre-update backup when the instance asks for it.
	wireSteamJobs(deps, inst, runner, steamClient, checker, backups.PreUpdateBackup)

	// WP-07: schedules drive backups, updates and restarts through hooks.
	hooks := scheduler.Hooks{
		Backup: func(ctx context.Context, id, by string) (*domain.Job, error) {
			return backups.EnqueueBackup(ctx, id, domain.BackupScheduled, "", by)
		},
		Update: func(ctx context.Context, id, by string, stopIfRunning bool) (*domain.Job, error) {
			return deps.Steam.EnqueueUpdate(ctx, id, by, stopIfRunning)
		},
		UpdateAvailable: func(ctx context.Context, id string) (bool, error) {
			info, err := deps.Steam.CheckUpdate(ctx, id)
			if err != nil {
				return false, err
			}
			return info.UpdateAvailable, nil
		},
		// deps.Agent is wired after the scheduler; the closure reads it at
		// call time (a restart), by which point it is set.
		Broadcast: func(ctx context.Context, id, message string) error {
			if deps.Agent == nil {
				return nil
			}
			res, err := deps.Agent.Command(ctx, id, domain.AgentCommandRequest{
				Command: "broadcast", Message: message, Style: "center",
			})
			if err != nil {
				return err
			}
			if !res.OK {
				return fmt.Errorf("agent refused broadcast: %s", res.Message)
			}
			return nil
		},
	}
	wireScheduler(ctx, deps, inst, runner, playersMgr, hooks)

	// WP-08: BepInEx, Thunderstore and mod config editing.
	// Server plugin: config writer, poller and the package the mods service
	// installs together with BepInEx.
	agentBundle := wireAgent(ctx, deps, inst)

	if err := wireMods(ctx, deps, inst, runner, agentBundle); err != nil {
		return fmt.Errorf("mods: %w", err)
	}

	// WP-30: manager self-upgrade. Auto-upgrade only fires when no player is
	// online on any instance.
	playersEverywhere := func(ctx context.Context) (int, error) {
		list, err := inst.List(ctx)
		if err != nil {
			return 0, err
		}
		total := 0
		for _, in := range list {
			if n, ok := playersMgr.PlayersOnline(ctx, in.ID); ok {
				total += n
			}
		}
		return total, nil
	}
	// systemd starts enabled valheim@<id> units at boot by itself; the direct
	// supervisor has no such memory, so when the manager is the Windows
	// service that owns the game processes it starts flagged instances here.
	startAutostartInstances(ctx, deps, inst)

	if err := wireSelfUpdate(ctx, deps, runner, playersEverywhere); err != nil {
		return fmt.Errorf("self-update: %w", err)
	}
	return nil
}
