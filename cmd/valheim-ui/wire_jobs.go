package main

import (
	"context"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
	"github.com/jonasthim/valheim-server-ui/internal/steam"
)

// wireJobs constructs the job runner and the steam client/service (WP-04),
// starts the runner's background loop bound to ctx, and attaches deps.Jobs
// and deps.Steam.
//
// wire.go should call this from wireServices early (it needs only deps.DB,
// deps.Bus, deps.Cfg and deps.Log, all set before wireServices runs), since
// later packages (WP-05 install/update jobs, WP-06 backups, WP-07 scheduler,
// WP-08 mods) receive the returned *jobs.Runner/*steam.Client — or just
// deps.Jobs/deps.Steam — as a constructor argument.
//
// The update checker (steam.NewUpdateChecker) is deliberately NOT started
// here: it needs deps.Instances.ListInstalled (WP-02) and the configured
// check interval from deps.Settings (WP-01), neither of which exist yet at
// this point in wireServices. Once both are wired, start it with e.g.:
//
//	checker := steam.NewUpdateChecker(
//	    steamClient,
//	    func() time.Duration {
//	        s, _ := deps.Settings.Get(ctx)
//	        return time.Duration(s.Updates.CheckIntervalMinutes) * time.Minute
//	    },
//	    func(ctx context.Context) ([]steam.InstalledRef, error) { ... via deps.Instances ... },
//	    func(ctx context.Context, instanceID, installed, latest string, at time.Time) error { ... via deps.Instances ... },
//	    deps.Bus,
//	)
//	go checker.Run(ctx)
//	deps.Steam = steam.NewService(steamClient, checker) // replaces the checker-less adapter wireJobs installed
func wireJobs(ctx context.Context, deps *api.Deps) (*jobs.Runner, *steam.Client, error) {
	runner := jobs.New(deps.DB, deps.Bus, deps.Cfg.JobsDir(), deps.Log)
	go func() {
		if err := runner.Start(ctx); err != nil {
			deps.Log.Error("jobs: runner stopped unexpectedly", "err", err)
		}
	}()

	client := steam.New(deps.Cfg.SteamCMDPath, deps.Log)

	deps.Jobs = runner
	deps.Steam = steam.NewService(client, nil)
	return runner, client, nil
}
