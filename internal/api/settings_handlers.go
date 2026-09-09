package api

import (
	"net/http"

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
		out, err := d.Settings.Put(r.Context(), in)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "settings.update", "", "", map[string]any{
			"local_login_enabled": out.Auth.LocalLoginEnabled,
			"oidc_enabled":        out.Auth.OIDC.Enabled,
		})
		WriteJSON(w, http.StatusOK, d.Settings.Redacted(out))
	}
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
