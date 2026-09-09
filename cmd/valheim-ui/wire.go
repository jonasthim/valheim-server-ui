package main

import (
	"context"

	"github.com/jonasthim/valheim-server-ui/internal/api"
)

// wireServices constructs feature services and attaches them to deps.
// Each work package adds its own block here, in dependency order:
//
//	WP-01: auth (deps.Auth, deps.Audit, deps.Users, deps.Settings)
//	WP-04: jobs runner, steam client, update ticker (deps.Jobs, deps.Steam)
//	WP-02: instance service (deps.Instances) — needs Supervisor, Bus, DB
//	WP-03: log tailers, players (deps.Players) + status enrichers
//	WP-06: backups (deps.Backups)   WP-07: scheduler (deps.Schedules)
//	WP-08: mods + thunderstore (deps.Mods, deps.Thunderstore)
//
// Keep this file as the single place where concrete types meet.
func wireServices(ctx context.Context, deps *api.Deps) error {
	_ = ctx
	_ = deps
	return nil
}
