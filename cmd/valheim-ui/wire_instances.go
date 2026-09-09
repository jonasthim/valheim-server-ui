package main

import (
	"context"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
)

// wireInstances constructs the instance service (WP-02) and attaches it to
// deps, then starts its background status-polling loop bound to ctx.
//
// wire.go calls this from wireServices, after the Supervisor and events.Bus
// dependencies are set on deps but ideally before any package that wants to
// call deps.Instances.RegisterEnricher during its own wiring.
//
//nolint:unused // wired in from wire.go by whoever integrates WP-02 (see docs/WORKPLAN.md); not called from this package's own code
func wireInstances(ctx context.Context, deps *api.Deps) error {
	svc := instance.New(deps.DB, deps.Bus, deps.Supervisor, deps.Cfg, deps.Log)
	deps.Instances = svc
	go svc.Poll(ctx)
	return nil
}
