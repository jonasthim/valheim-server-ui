package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerAuditRoutes mounts GET /audit (docs/openapi.yaml → audit tag, admin only).
func registerAuditRoutes(r chi.Router, d *Deps) {
	r.Route("/audit", func(r chi.Router) {
		r.Use(RequireRole(domain.RoleAdmin))
		r.Get("/", listAuditHandler(d))
	})
}

const (
	defaultAuditLimit = 100
	maxAuditLimit     = 500
)

func listAuditHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Audit == nil {
			WriteJSON(w, http.StatusOK, map[string]any{"entries": []domain.AuditEntry{}})
			return
		}
		q := r.URL.Query()
		instance := q.Get("instance")
		username := q.Get("user")

		limit := defaultAuditLimit
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				WriteValidation(w, domain.FieldError{Field: "limit", Message: "must be a positive integer"})
				return
			}
			limit = n
		}
		if limit > maxAuditLimit {
			limit = maxAuditLimit
		}

		var before int64
		if v := q.Get("before"); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				WriteValidation(w, domain.FieldError{Field: "before", Message: "must be a non-negative integer"})
				return
			}
			before = n
		}

		entries, err := d.Audit.List(r.Context(), instance, username, limit, before)
		if err != nil {
			WriteError(w, err)
			return
		}
		if entries == nil {
			entries = []domain.AuditEntry{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"entries": entries})
	}
}
