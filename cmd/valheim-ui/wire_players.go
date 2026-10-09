package main

import (
	"context"
	"fmt"
	"log/slog"

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
func wirePlayers(
	ctx context.Context,
	deps *api.Deps,
	list func(ctx context.Context) ([]players.InstanceRef, error),
	paths func(string) domain.InstancePaths,
	exists func(context.Context, string) (bool, error),
	register func(domain.StatusEnricher),
) (*players.Manager, error) {
	store := players.NewSQLStore(deps.DB)

	mgr, err := players.NewManager(ctx, deps.Bus, store, deps.Log, list, paths)
	if err != nil {
		return nil, fmt.Errorf("wire players: %w", err)
	}

	svc := players.NewService(mgr, store, paths, exists)
	deps.Players = svc
	// Older versions wrote UI comments on the ID line. Repair those files
	// now so Valheim can recognize affected admins without a UI visit.
	if list != nil {
		refs, err := list(ctx)
		if err == nil {
			log := deps.Log
			if log == nil {
				log = slog.Default()
			}
			for _, ref := range refs {
				for _, kind := range []domain.ListKind{domain.ListAdmin, domain.ListBanned, domain.ListPermitted} {
					if _, err := svc.GetList(ctx, ref.ID, kind); err != nil {
						log.Warn("players: legacy list comment repair", "instance", ref.ID, "kind", kind, "err", err)
					}
				}
			}
		} else if deps.Log != nil {
			deps.Log.Warn("players: legacy list comment repair skipped", "err", err)
		}
	}
	if register != nil {
		register(mgr)
	}
	return mgr, nil
}
