package main

import (
	"context"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
	"github.com/jonasthim/valheim-server-ui/internal/scheduler"
)

// wireScheduler constructs the WP-07 scheduler service, attaches deps.Schedules
// and starts its cron engine (Run) bound to ctx.
//
// hooks.Backup and hooks.Update are expected to be wired to the backup
// service's EnqueueBackup (kind "scheduled", WP-06) and the Steam update job
// (WP-05) respectively; hooks.UpdateAvailable to its CheckUpdate. The
// scheduler package never imports internal/backup or
// internal/instance/jobs.go directly — those actions arrive only through
// hooks — so it can be developed and tested independently of both.
//
// Call this from wireServices after wireJobs (needs runner), wireInstances
// (needs inst) and whichever of WP-05/WP-06 supply hooks.Update/hooks.Backup
// are ready; deps.Players (domain.PlayerCounter) must also be set (WP-03).
//
//nolint:unused // wired in from wire.go by whoever integrates WP-07 (see docs/WORKPLAN.md); not called from this package's own code yet
func wireScheduler(ctx context.Context, deps *api.Deps, inst *instance.Service, runner *jobs.Runner, players domain.PlayerCounter, hooks scheduler.Hooks) *scheduler.Service {
	svc := scheduler.New(deps.DB, inst, runner, players, hooks, deps.Log)
	deps.Schedules = svc
	go func() {
		if err := svc.Run(ctx); err != nil {
			deps.Log.Error("scheduler: run stopped unexpectedly", "err", err)
		}
	}()
	return svc
}
