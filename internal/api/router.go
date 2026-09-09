// Package api wires the HTTP surface: chi router, middleware, JSON helpers and
// one *_handlers.go file per resource. Handlers call services; they contain no
// business logic beyond decoding, role checks and audit records.
package api

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// Authenticator is implemented by the auth package (WP-01).
type Authenticator interface {
	// Authenticate resolves the session cookie and stores the user in the context
	// via WithUser. Unauthenticated requests pass through with no user.
	Authenticate(next http.Handler) http.Handler
}

// Auditor records mutating actions (WP-01). A nil Auditor is a no-op.
type Auditor interface {
	Record(r *http.Request, action, instanceID, target string, details map[string]any)
}

// Deps is everything handlers may need. Wave-1 packages add their services here
// as exported fields; keep additions minimal and documented.
type Deps struct {
	Cfg        config.Config
	Log        *slog.Logger
	DB         *sql.DB
	Bus        *events.Bus
	Supervisor supervisor.Supervisor
	Version    string
	Commit     string
	StartedAt  time.Time

	Auth  Authenticator // nil only when Cfg.DevNoAuth (fake admin injected)
	Audit Auditor

	// Populated by feature packages (see WORKPLAN.md file ownership):
	Instances    InstanceService     // WP-02
	Jobs         JobService          // WP-04
	Users        UserService         // WP-01
	Settings     SettingsService     // WP-01
	Players      PlayerService       // WP-03
	Backups      BackupService       // WP-06
	Schedules    ScheduleService     // WP-07
	Mods         ModService          // WP-08
	Thunderstore ThunderstoreService // WP-08
	Steam        SteamService        // WP-04/05
}

// NewRouter builds the full HTTP handler: /api/v1 plus the embedded SPA.
func NewRouter(d *Deps, spa http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(requestLogger(d.Log))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(5 * time.Minute))

	r.Route("/api/v1", func(api chi.Router) {
		if d.Auth != nil {
			api.Use(d.Auth.Authenticate)
		} else {
			api.Use(devFakeAdmin(d))
		}
		api.Use(csrfGuard(d.Cfg))

		registerAuthRoutes(api, d)
		registerUserRoutes(api, d)
		registerSettingsRoutes(api, d)
		registerSystemRoutes(api, d)
		registerInstanceRoutes(api, d)
		registerInstanceJobRoutes(api, d)
		registerPlayerRoutes(api, d)
		registerWorldRoutes(api, d)
		registerBackupRoutes(api, d)
		registerScheduleRoutes(api, d)
		registerModRoutes(api, d)
		registerThunderstoreRoutes(api, d)
		registerJobRoutes(api, d)
		registerEventRoutes(api, d)
		registerAuditRoutes(api, d)

		api.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			WriteError(w, domain.NotFound("route"))
		})
	})

	if spa != nil {
		r.Handle("/*", spa)
	}
	return r
}

// devFakeAdmin injects a synthetic admin when no Authenticator is configured.
// Only reachable when config.DevNoAuth is true (serve refuses otherwise).
func devFakeAdmin(d *Deps) func(http.Handler) http.Handler {
	fake := &domain.User{ID: 0, Username: "dev", DisplayName: "Dev (no auth)", Role: domain.RoleAdmin}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), fake)))
		})
	}
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			if r.URL.Path == "/api/v1/events" {
				return // long-lived
			}
			log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(),
				"bytes", ww.BytesWritten(), "dur", time.Since(start).Round(time.Millisecond), "ip", r.RemoteAddr)
		})
	}
}
