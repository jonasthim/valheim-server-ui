package main

import (
	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
	"github.com/jonasthim/valheim-server-ui/internal/steam"
)

// wireSteamJobs attaches the WP-05 install/update-job implementation of
// api.SteamService to deps, replacing the checker-less "not wired" adapter
// wireJobs installs. pre is WP-06's pre-update backup hook and may be nil
// until that package lands (the update job then skips the backup step).
//
// Call this from wireServices after wireInstances (needs svc), wireJobs
// (needs runner/client) and the steam.UpdateChecker construction (needs
// checker) — see cmd/valheim-ui/wire.go and wire_jobs.go.
//
//nolint:unused // wired in from wire.go by whoever integrates WP-05 (see docs/WORKPLAN.md); not called from this package's own code yet
func wireSteamJobs(deps *api.Deps, svc *instance.Service, runner *jobs.Runner, client *steam.Client, checker *steam.UpdateChecker, pre instance.PreUpdateBackupFunc) {
	deps.Steam = instance.NewSteamJobs(svc, runner, client, checker, pre, deps.Log)
}
