package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerNotifyRoutes mounts GET /notifications and POST
// /settings/notifications/{channelId}/test (docs/openapi.yaml → settings
// tag, F-1.1, admin only).
func registerNotifyRoutes(r chi.Router, d *Deps) {
	guard := requireService(func() bool { return d.Notify != nil }, "notifications service not configured")
	r.With(RequireRole(domain.RoleAdmin), guard).Get("/notifications", listNotificationsHandler(d))
	r.With(RequireRole(domain.RoleAdmin), guard).Post("/settings/notifications/{channelId}/test", notifyTestHandler(d))
}

const (
	defaultNotificationLimit = 100
	maxNotificationLimit     = 500
)

// listNotificationsHandler is GET /notifications?limit=&before=.
func listNotificationsHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		limit := defaultNotificationLimit
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				WriteValidation(w, domain.FieldError{Field: "limit", Message: "must be a positive integer"})
				return
			}
			limit = n
		}
		if limit > maxNotificationLimit {
			limit = maxNotificationLimit
		}

		var before *time.Time
		if v := q.Get("before"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				WriteValidation(w, domain.FieldError{Field: "before", Message: "must be an RFC3339 timestamp"})
				return
			}
			before = &t
		}

		entries, err := d.Notify.Log(r.Context(), limit, before)
		if err != nil {
			WriteError(w, err)
			return
		}
		if entries == nil {
			entries = []domain.NotificationLogEntry{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"entries": entries})
	}
}

// notifyTestHandler is POST /settings/notifications/{channelId}/test.
func notifyTestHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID := chi.URLParam(r, "channelId")
		if err := d.Notify.Test(r.Context(), channelID); err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "notifications.test", "", channelID, nil)
		WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
