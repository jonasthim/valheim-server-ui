package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerSettingsRoutes mounts /settings* (docs/openapi.yaml → settings tag, admin only).
func registerSettingsRoutes(r chi.Router, d *Deps) {
	r.Route("/settings", func(r chi.Router) {
		r.Use(RequireRole(domain.RoleAdmin))
		r.Use(requireService(func() bool { return d.Settings != nil }, "settings service not configured"))
		r.Get("/", getSettingsHandler(d))
		r.Put("/", putSettingsHandler(d))
		r.Post("/oidc/test", testOIDCHandler(d))
	})
}

func getSettingsHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := d.Settings.Get(r.Context())
		if err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, d.Settings.Redacted(s))
	}
}

func putSettingsHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in domain.Settings
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, err)
			return
		}
		prev, prevErr := d.Settings.Get(r.Context())
		out, err := d.Settings.Put(r.Context(), in)
		if err != nil {
			WriteError(w, err)
			return
		}
		details := map[string]any{
			"local_login_enabled": out.Auth.LocalLoginEnabled,
			"oidc_enabled":        out.Auth.OIDC.Enabled,
		}
		if prevErr == nil {
			// client_secret and every notification channel's secret are
			// masked by auditDiff itself (sensitiveKeys["secret"]); a
			// channel's url is masked here too, since for discord/slack/
			// telegram it embeds a bearer credential just like secret does
			// (see auth.channelURLEmbedsSecret) — mask it for every type so
			// the audit log stays simple to reason about.
			details["changes"] = maskNotifyChannelURLs(auditDiff(prev, out))
		}
		d.audit(r, "settings.update", "", "", details)
		WriteJSON(w, http.StatusOK, d.Settings.Redacted(out))
	}
}

// maskNotifyChannelURLs blanks the From/To of any changes entry whose path
// is a notification channel's url (notifications.channels.<N>.url). Unlike
// auth.oidc.client_secret and a channel's own secret, the url is not always
// sensitive (ntfy/webhook/email URLs are plain addresses), but discord/slack
// webhook paths and a telegram bot token live in the URL itself, so every
// channel's url is masked here for simplicity — see the security note on
// auth.channelURLEmbedsSecret.
func maskNotifyChannelURLs(changes []auditChange) []auditChange {
	for i := range changes {
		if !strings.HasPrefix(changes[i].Path, "notifications.channels.") || !strings.HasSuffix(changes[i].Path, ".url") {
			continue
		}
		if changes[i].From != nil {
			changes[i].From = maskedValue
		}
		if changes[i].To != nil {
			changes[i].To = maskedValue
		}
	}
	return changes
}

type oidcTestResponse struct {
	OK                    bool   `json:"ok"`
	Issuer                string `json:"issuer,omitempty"`
	AuthorizationEndpoint string `json:"authorization_endpoint,omitempty"`
	Error                 string `json:"error,omitempty"`
}

func testOIDCHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in domain.OIDCSettings
		if err := DecodeOptionalJSON(r, &in); err != nil {
			WriteError(w, err)
			return
		}
		ok, issuer, authEndpoint, errMsg := d.Settings.TestOIDC(r.Context(), in.IssuerURL)
		WriteJSON(w, http.StatusOK, oidcTestResponse{OK: ok, Issuer: issuer, AuthorizationEndpoint: authEndpoint, Error: errMsg})
	}
}
