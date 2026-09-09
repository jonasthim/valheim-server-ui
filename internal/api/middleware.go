package api

import (
	"net/http"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// requireService is a small guard for routes whose backing service is
// populated by WP-01's wiring; it turns a nil dependency into a clean 500
// instead of a panic if the binary is ever assembled without it.
func requireService(present func() bool, msg string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !present() {
				WriteError(w, domain.E(domain.CodeInternal, msg))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
