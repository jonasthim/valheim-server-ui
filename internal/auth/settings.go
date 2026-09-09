package auth

import (
	"context"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

var _ api.SettingsService = (*Settings)(nil)

// Settings implements api.SettingsService. It is a separate type from
// Service because api.UserService and api.SettingsService both declare a
// Get method with different signatures, which one type cannot satisfy at
// once; OnChange is wired to Service.invalidateOIDC so that a successful Put
// rebuilds the cached OIDC provider on next use.
type Settings struct {
	repo     *db.SettingsRepo
	cfg      config.Config
	onChange func()
}

// NewSettings constructs the settings service. onChange (may be nil) is
// called after every successful Put.
func NewSettings(repo *db.SettingsRepo, cfg config.Config, onChange func()) *Settings {
	return &Settings{repo: repo, cfg: cfg, onChange: onChange}
}

// Get returns the stored settings (or defaults when none have been saved yet).
func (s *Settings) Get(ctx context.Context) (domain.Settings, error) {
	return s.repo.Get(ctx)
}

// Put validates and stores settings. An empty auth.oidc.client_secret keeps
// the previously stored secret. Successful writes invalidate the cached OIDC
// provider so it is rebuilt from the new configuration on next use.
func (s *Settings) Put(ctx context.Context, in domain.Settings) (domain.Settings, error) {
	current, err := s.repo.Get(ctx)
	if err != nil {
		return domain.Settings{}, err
	}

	out := in
	oidc := &out.Auth.OIDC

	if oidc.ClientSecret == "" {
		oidc.ClientSecret = current.Auth.OIDC.ClientSecret
	}
	if oidc.ProviderName == "" {
		oidc.ProviderName = "SSO"
	}
	if oidc.GroupsClaim == "" {
		oidc.GroupsClaim = "groups"
	}
	if len(oidc.Scopes) == 0 {
		oidc.Scopes = []string{"openid", "profile", "email", "groups"}
	}
	if oidc.DefaultRole == "" {
		oidc.DefaultRole = string(domain.RoleViewer)
	}
	if oidc.RoleMapping == nil {
		oidc.RoleMapping = map[string]domain.Role{}
	}
	oidc.RedirectURI = "" // computed, never stored

	var fields []domain.FieldError
	if oidc.Enabled {
		if oidc.IssuerURL == "" {
			fields = append(fields, domain.FieldError{Field: "auth.oidc.issuer_url", Message: "required when OIDC is enabled"})
		}
		if oidc.ClientID == "" {
			fields = append(fields, domain.FieldError{Field: "auth.oidc.client_id", Message: "required when OIDC is enabled"})
		}
		if oidc.ClientSecret == "" {
			fields = append(fields, domain.FieldError{Field: "auth.oidc.client_secret", Message: "required when OIDC is enabled"})
		}
	}
	switch oidc.DefaultRole {
	case string(domain.RoleViewer), string(domain.RoleOperator), string(domain.RoleAdmin), "deny":
	default:
		fields = append(fields, domain.FieldError{Field: "auth.oidc.default_role", Message: "must be viewer, operator, admin or deny"})
	}
	for group, role := range oidc.RoleMapping {
		if !role.Valid() {
			fields = append(fields, domain.FieldError{Field: "auth.oidc.role_mapping." + group, Message: "invalid role"})
		}
	}
	if out.Updates.CheckIntervalMinutes < 0 {
		fields = append(fields, domain.FieldError{Field: "updates.check_interval_minutes", Message: "must be >= 0"})
	}
	if out.Thunderstore.IndexRefreshHours < 1 {
		fields = append(fields, domain.FieldError{Field: "thunderstore.index_refresh_hours", Message: "must be >= 1"})
	}
	if len(fields) > 0 {
		return domain.Settings{}, domain.Validation(fields)
	}

	if err := s.repo.Put(ctx, out); err != nil {
		return domain.Settings{}, err
	}
	if s.onChange != nil {
		s.onChange()
	}
	return out, nil
}

// TestOIDC performs discovery-only validation of an issuer URL, used by
// POST /settings/oidc/test. It ignores the rest of the payload (client
// credentials are not needed to discover the issuer metadata).
func (s *Settings) TestOIDC(ctx context.Context, issuerURL string) (ok bool, issuer, authEndpoint, errMsg string) {
	return TestOIDCDiscovery(ctx, issuerURL)
}

// Redacted blanks the OIDC client secret and fills in the computed redirect
// URI, per the OpenAPI contract for API responses.
func (s *Settings) Redacted(in domain.Settings) domain.Settings {
	out := in
	out.Auth.OIDC.ClientSecret = ""
	out.Auth.OIDC.RedirectURI = s.cfg.BaseURL + oidcCallbackPath
	return out
}
