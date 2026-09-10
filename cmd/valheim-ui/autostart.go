package main

import (
	"context"
	"runtime"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
)

// startAutostartInstances starts every installed instance flagged autostart
// when the direct supervisor is the one keeping instances alive across boots,
// which is the case for the Windows service. On Linux the direct supervisor
// is a development tool and systemd owns autostart, so this is a no-op.
func startAutostartInstances(ctx context.Context, deps *api.Deps, inst *instance.Service) {
	if deps.Cfg.Supervisor != "direct" || runtime.GOOS != "windows" {
		return
	}
	go func() {
		list, err := inst.List(ctx)
		if err != nil {
			deps.Log.Warn("autostart: list instances", "err", err)
			return
		}
		for _, in := range list {
			if !in.Status.Autostart || in.Status.State != domain.StateStopped {
				continue
			}
			if _, err := inst.Start(ctx, in.ID); err != nil {
				deps.Log.Warn("autostart: start instance", "instance", in.ID, "err", err)
				continue
			}
			deps.Log.Info("autostart: instance started", "instance", in.ID)
		}
	}()
}
