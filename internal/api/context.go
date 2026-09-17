package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

type ctxKey int

const (
	userKey ctxKey = iota
	tokenAuthKey
)

// WithUser stores the authenticated user in the context.
func WithUser(ctx context.Context, u *domain.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// UserFrom returns the authenticated user or nil.
func UserFrom(ctx context.Context) *domain.User {
	u, _ := ctx.Value(userKey).(*domain.User)
	return u
}

// WithTokenAuth marks the context as authenticated via a personal API bearer
// token (F-2.5) rather than the session cookie, so csrfGuard can skip its
// browser-only checks and handlers can refuse token-authenticated callers
// from managing tokens themselves.
func WithTokenAuth(ctx context.Context) context.Context {
	return context.WithValue(ctx, tokenAuthKey, true)
}

// IsTokenAuth reports whether the request was authenticated via a bearer API
// token rather than the session cookie (F-2.5).
func IsTokenAuth(ctx context.Context) bool {
	v, _ := ctx.Value(tokenAuthKey).(bool)
	return v
}

// RequireRole rejects requests whose user is missing or below min.
func RequireRole(min domain.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFrom(r.Context())
			switch {
			case u == nil:
				WriteError(w, domain.E(domain.CodeUnauthorized, "authentication required"))
			case u.Disabled:
				WriteError(w, domain.E(domain.CodeAccountDisabled, "account disabled"))
			case !u.Role.AtLeast(min):
				WriteError(w, domain.Ef(domain.CodeForbidden, "requires role %s", min))
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

// CSRFHeader must be present on every mutating request.
const CSRFHeader = "X-Requested-With"
const CSRFHeaderValue = "valheim-ui"

// csrfGuard enforces the custom header and, when base_url is set, an Origin or
// Referer matching it, for all non-safe methods. The OIDC callback is a GET.
func csrfGuard(cfg config.Config) func(http.Handler) http.Handler {
	var baseHost string
	if u, err := url.Parse(cfg.BaseURL); err == nil {
		baseHost = strings.ToLower(u.Host)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			if IsTokenAuth(r.Context()) {
				// A bearer API token is never sent by a browser tab an
				// attacker's page could ride along with, so the CSRF header
				// and Origin/Referer checks (which exist for the cookie) do
				// not apply (F-2.5).
				next.ServeHTTP(w, r)
				return
			}
			if r.Header.Get(CSRFHeader) != CSRFHeaderValue {
				WriteError(w, domain.E(domain.CodeForbidden, "missing "+CSRFHeader+" header"))
				return
			}
			if baseHost != "" {
				src := r.Header.Get("Origin")
				if src == "" {
					src = r.Header.Get("Referer")
				}
				if src != "" {
					if u, err := url.Parse(src); err != nil || strings.ToLower(u.Host) != baseHost {
						WriteError(w, domain.E(domain.CodeForbidden, "cross-origin request rejected"))
						return
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// InstanceID reads and validates the {instanceId} URL parameter.
func InstanceID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "instanceId")
	if !domain.InstanceIDPattern.MatchString(id) {
		return "", domain.E(domain.CodeValidationFailed, "invalid instance id")
	}
	return id, nil
}

// ClientIP returns the best-effort remote address (RealIP middleware applied).
func ClientIP(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return strings.Trim(host, "[]")
}
