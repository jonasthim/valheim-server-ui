// Package api wires the HTTP surface: chi router, middleware, JSON helpers and
// one *_handlers.go file per resource. Handlers call services; they contain no
// business logic beyond decoding, role checks and audit records.
package api

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// Authenticator is implemented by the auth package (WP-01). Beyond the
// Authenticate middleware, it also carries the login/setup/logout/OIDC flow
// so auth_handlers.go can drive it through this interface without the api
// package importing internal/auth (which itself depends on api.WithUser /
// api.ClientIP, and would otherwise create an import cycle).
type Authenticator interface {
	// Authenticate resolves the session cookie and stores the user in the context
	// via WithUser. Unauthenticated requests pass through with no user.
	Authenticate(next http.Handler) http.Handler
	// Setup creates the first admin account (only while no users exist) and
	// logs it in by setting the session cookie.
	Setup(ctx context.Context, w http.ResponseWriter, r *http.Request, username, password, displayName, email string) (*domain.User, error)
	// Login authenticates a local account and sets the session cookie.
	Login(ctx context.Context, w http.ResponseWriter, r *http.Request, username, password string) (*domain.User, error)
	// Logout deletes the current session (if any) and clears the cookie.
	Logout(ctx context.Context, w http.ResponseWriter, r *http.Request)
	// OIDCLogin starts the OIDC authorization code + PKCE flow.
	OIDCLogin(w http.ResponseWriter, r *http.Request)
	// OIDCCallback completes the OIDC flow and redirects to the SPA.
	OIDCCallback(w http.ResponseWriter, r *http.Request)
}

// Auditor records mutating actions (WP-01). A nil Auditor is a no-op.
type Auditor interface {
	Record(r *http.Request, action, instanceID, target string, details map[string]any)
	// List returns audit entries newest-first for GET /audit, optionally
	// filtered by instance and/or username, paged with a "before id" cursor
	// (0 = no cursor).
	List(ctx context.Context, instanceID, username string, limit int, before int64) ([]domain.AuditEntry, error)
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
	r.Use(realIP)
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

// realIP trusts X-Forwarded-For / X-Real-IP only when the direct peer is a
// loopback address, which is the documented deployment (reverse proxy on the
// same host). chi's RealIP is deliberately not used: it trusts every peer.
func realIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.RemoteAddr
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			fwd := r.Header.Get("X-Real-IP")
			if fwd == "" {
				if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
					parts := strings.Split(xff, ",")
					fwd = strings.TrimSpace(parts[len(parts)-1])
				}
			}
			if fwd != "" && net.ParseIP(fwd) != nil {
				r.RemoteAddr = net.JoinHostPort(fwd, "0")
			}
		}
		next.ServeHTTP(w, r)
	})
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
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
