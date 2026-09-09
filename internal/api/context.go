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

const userKey ctxKey = iota

// WithUser stores the authenticated user in the context.
func WithUser(ctx context.Context, u *domain.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// UserFrom returns the authenticated user or nil.
func UserFrom(ctx context.Context) *domain.User {
	u, _ := ctx.Value(userKey).(*domain.User)
	return u
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
