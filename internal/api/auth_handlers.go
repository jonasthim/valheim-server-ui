package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerAuthRoutes mounts /auth/* (docs/openapi.yaml → auth tag).
func registerAuthRoutes(r chi.Router, d *Deps) {
	r.Route("/auth", func(r chi.Router) {
		r.Get("/status", authStatusHandler(d))
		r.Post("/setup", authSetupHandler(d))
		r.Post("/login", authLoginHandler(d))
		r.Get("/oidc/login", oidcLoginHandler(d))
		r.Get("/oidc/callback", oidcCallbackHandler(d))

		r.Group(func(r chi.Router) {
			r.Use(RequireRole(domain.RoleViewer))
			r.Post("/logout", authLogoutHandler(d))
			r.Get("/me", authMeHandler(d))
			r.Put("/password", authChangePasswordHandler(d))
		})
	})
}

type authStatus struct {
	NeedsSetup        bool   `json:"needs_setup"`
	LocalLoginEnabled bool   `json:"local_login_enabled"`
	OIDCEnabled       bool   `json:"oidc_enabled"`
	OIDCProviderName  string `json:"oidc_provider_name,omitempty"`
	AppVersion        string `json:"app_version,omitempty"`
}

func authStatusHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := authStatus{AppVersion: d.Version, LocalLoginEnabled: true}
		if d.Users != nil {
			n, err := d.Users.Count(r.Context())
			if err != nil {
				WriteError(w, err)
				return
			}
			out.NeedsSetup = n == 0
		}
		if d.Settings != nil {
			settings, err := d.Settings.Get(r.Context())
			if err != nil {
				WriteError(w, err)
				return
			}
			out.LocalLoginEnabled = settings.Auth.LocalLoginEnabled
			out.OIDCEnabled = settings.Auth.OIDC.Enabled
			if out.OIDCEnabled {
				out.OIDCProviderName = settings.Auth.OIDC.ProviderName
			}
		}
		WriteJSON(w, http.StatusOK, out)
	}
}

type setupRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

func authSetupHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Auth == nil {
			WriteError(w, domain.E(domain.CodeInternal, "auth service not configured"))
			return
		}
		var in setupRequest
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, err)
			return
		}
		usr, err := d.Auth.Setup(r.Context(), w, r, in.Username, in.Password, in.DisplayName, in.Email)
		if err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "auth.setup", "", usr.Username, nil)
		WriteJSON(w, http.StatusCreated, map[string]any{"user": usr})
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func authLoginHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Auth == nil {
			WriteError(w, domain.E(domain.CodeInternal, "auth service not configured"))
			return
		}
		var in loginRequest
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, err)
			return
		}
		usr, err := d.Auth.Login(r.Context(), w, r, in.Username, in.Password)
		if err != nil {
			d.audit(r, "auth.login.failed", "", in.Username, nil)
			WriteError(w, err)
			return
		}
		d.audit(r, "auth.login", "", usr.Username, nil)
		WriteJSON(w, http.StatusOK, map[string]any{"user": usr})
	}
}

func authLogoutHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Auth == nil {
			WriteError(w, domain.E(domain.CodeInternal, "auth service not configured"))
			return
		}
		usr := UserFrom(r.Context())
		d.Auth.Logout(r.Context(), w, r)
		if usr != nil {
			d.audit(r, "auth.logout", "", usr.Username, nil)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func authMeHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		usr := UserFrom(r.Context())
		if usr == nil {
			WriteError(w, domain.E(domain.CodeUnauthorized, "authentication required"))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"user": usr})
	}
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func authChangePasswordHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Users == nil {
			WriteError(w, domain.E(domain.CodeInternal, "user service not configured"))
			return
		}
		usr := UserFrom(r.Context())
		if usr == nil {
			WriteError(w, domain.E(domain.CodeUnauthorized, "authentication required"))
			return
		}
		var in changePasswordRequest
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, err)
			return
		}
		if err := d.Users.ChangePassword(r.Context(), usr.ID, in.CurrentPassword, in.NewPassword); err != nil {
			WriteError(w, err)
			return
		}
		d.audit(r, "user.password.change", "", usr.Username, nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func oidcLoginHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Auth == nil {
			WriteError(w, domain.E(domain.CodeInternal, "auth service not configured"))
			return
		}
		d.Auth.OIDCLogin(w, r)
	}
}

func oidcCallbackHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Auth == nil {
			http.Redirect(w, r, "/login?error=oidc_disabled", http.StatusFound)
			return
		}
		// OIDCCallback redirects on both success and failure and does not
		// return the resulting user, so it records its own audit entry
		// (via the Auditor injected at construction) rather than the
		// handler doing it after the fact.
		d.Auth.OIDCCallback(w, r)
	}
}
