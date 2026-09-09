package main

import (
	"context"
	"fmt"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/players"
)

// wirePlayers constructs the players.Manager and players.Service (WP-03),
// attaches deps.Players, registers the players status enricher, and starts
// the manager's tail/poll/bus-subscribe loops bound to ctx.
//
// wire.go should call this from wireServices after deps.DB, deps.Bus and
// deps.Log are set and after the instance service (WP-02) exists, since the
// callbacks below are expected to be built from it:
//
//	list   -> deps.Instances.List(ctx), mapped to []players.InstanceRef
//	        (ID, Paths, QueryPort = config.Port+1, Running = status running/starting)
//	paths  -> deps.Instances' path helper (domain.PathsFor(dataDir, id) or
//	        equivalent), used for the admin/banned/permitted list files
//	exists -> a lightweight existence check (e.g. via deps.Instances.Get)
//	register -> deps.Instances.RegisterEnricher (or however WP-02 exposes it)
//
// This lets players.Manager stay independent of the concrete instance
// package: it only ever sees the small InstanceRef/callback shapes defined
// in internal/players.
//
//nolint:unused // wired in from wire.go by whoever integrates WP-03 (see docs/WORKPLAN.md); not called from this package's own code yet
func wirePlayers(
	ctx context.Context,
	deps *api.Deps,
	list func(ctx context.Context) ([]players.InstanceRef, error),
	paths func(string) domain.InstancePaths,
	exists func(context.Context, string) (bool, error),
	register func(domain.StatusEnricher),
) error {
	store := players.NewSQLStore(deps.DB)

	mgr, err := players.NewManager(ctx, deps.Bus, store, deps.Log, list, paths)
	if err != nil {
		return fmt.Errorf("wire players: %w", err)
	}

	deps.Players = players.NewService(mgr, store, paths, exists)
	if register != nil {
		register(mgr)
	}
	return nil
}
