package main

import (
	"context"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/notify"
)

// wireNotify constructs the notification_log repo and the F-1.1 alerting
// Service, attaches deps.Notify, and starts its background loop (bus
// subscription + disk-space ticker) bound to ctx.
//
// wire.go should call this from wireServices once deps.DB, deps.Bus (set in
// serve.go before wireServices runs) and deps.Settings (WP-01, wireAuth) all
// exist, mirroring the settings closure in wire_selfupdate.go.
func wireNotify(ctx context.Context, deps *api.Deps) error {
	logRepo := db.NewNotificationLogRepo(deps.DB)
	settings := func() domain.NotifySettings {
		s, err := deps.Settings.Get(ctx)
		if err != nil {
			return domain.NotifySettings{}
		}
		return s.Notifications
	}
	svc := notify.New(deps.Bus, settings, deps.Cfg.DataDir, logRepo, deps.Log)

	deps.Notify = svc
	go svc.Run(ctx)
	return nil
}
