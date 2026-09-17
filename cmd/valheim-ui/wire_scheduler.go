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

func wireScheduler(ctx context.Context, deps *api.Deps, inst *instance.Service, runner *jobs.Runner, players domain.PlayerCounter, hooks scheduler.Hooks) *scheduler.Service {
	// hooks.Command forwards to the agent service's Command, discarding the
	// result, exactly like hooks.Broadcast (wire.go): a no-op when deps.Agent
	// is not configured. deps.Agent is wired after the scheduler; the closure
	// reads it at call time, by which point it is set.
	hooks.Command = func(ctx context.Context, id string, req domain.AgentCommandRequest) error {
		if deps.Agent == nil {
			return nil
		}
		_, err := deps.Agent.Command(ctx, id, req)
		return err
	}

	svc := scheduler.New(deps.DB, inst, runner, players, hooks, deps.Log)
	deps.Schedules = svc
	go func() {
		if err := svc.Run(ctx); err != nil {
			deps.Log.Error("scheduler: run stopped unexpectedly", "err", err)
		}
	}()
	return svc
}
