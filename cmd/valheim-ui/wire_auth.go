package main

import (
	"context"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/audit"
	"github.com/jonasthim/valheim-server-ui/internal/auth"
	"github.com/jonasthim/valheim-server-ui/internal/db"
)

// wireAuth constructs the WP-01 services (auth + audit + users + settings)
// and attaches them to deps. It is called from wireServices (wire.go), which
// the architect owns; this file only builds the WP-01 half of the graph.
//
// Construction order matters: the audit.Recorder is built first so it can be
// injected into auth.Service (auth.WithAuditor) — the OIDC callback records
// its own "auth.login" audit entry because it redirects the browser and
// never hands the logged-in user back to an HTTP handler that could audit it
// afterwards. Every other WP-01 mutation is audited by its *_handlers.go
// caller instead, following the same pattern already used by WP-02/03/04.
//
// settings.Put (internal/auth.Settings.Put) invalidates the cached OIDC
// provider on every successful write, so a changed issuer/client/secret in
// Settings takes effect on the next OIDC login without a restart.
func wireAuth(ctx context.Context, deps *api.Deps) error {
	_ = ctx

	auditRepo := db.NewAuditRepo(deps.DB)
	recorder := audit.New(auditRepo, deps.Log)

	authSvc := auth.NewService(deps.DB, deps.Cfg, deps.Log, auth.WithAuditor(recorder))
	settingsRepo := db.NewSettingsRepo(deps.DB)
	settingsSvc := auth.NewSettings(settingsRepo, deps.Cfg, authSvc.InvalidateOIDC)

	deps.Audit = recorder
	deps.Auth = authSvc
	deps.Users = authSvc
	deps.Settings = settingsSvc
	return nil
}
