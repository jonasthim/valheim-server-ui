package auth

import (
	"context"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func newTestSettings(t *testing.T, cfg config.Config, onChange func()) *Settings {
	t.Helper()
	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	return NewSettings(db.NewSettingsRepo(sqldb), cfg, onChange)
}

func TestSettingsGetDefaults(t *testing.T) {
	s := newTestSettings(t, config.Config{}, nil)
	got, err := s.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := domain.DefaultSettings()
	if got.Auth.LocalLoginEnabled != want.Auth.LocalLoginEnabled {
		t.Fatalf("unexpected default local_login_enabled: %+v", got)
	}
	if got.Auth.OIDC.ProviderName != want.Auth.OIDC.ProviderName {
		t.Fatalf("unexpected default provider name: %+v", got.Auth.OIDC)
	}
}

func TestSettingsPutValidation(t *testing.T) {
	s := newTestSettings(t, config.Config{}, nil)
	ctx := context.Background()

	valid := domain.DefaultSettings()
	valid.Auth.OIDC.Enabled = true // missing issuer/client id/secret
	_, err := s.Put(ctx, valid)
	de := domain.AsError(err)
	if de.Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed, got %v (%v)", de.Code, err)
	}
	if len(de.Fields) == 0 {
		t.Fatalf("expected field errors, got none")
	}

	bad := domain.DefaultSettings()
	bad.Auth.OIDC.DefaultRole = "superuser"
	_, err = s.Put(ctx, bad)
	if domain.AsError(err).Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed for bad default_role, got %v", err)
	}

	bad2 := domain.DefaultSettings()
	bad2.Auth.OIDC.RoleMapping = map[string]domain.Role{"admins": "superadmin"}
	_, err = s.Put(ctx, bad2)
	if domain.AsError(err).Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed for bad role mapping, got %v", err)
	}

	bad3 := domain.DefaultSettings()
	bad3.Thunderstore.IndexRefreshHours = 0
	_, err = s.Put(ctx, bad3)
	if domain.AsError(err).Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed for index_refresh_hours=0, got %v", err)
	}
}

func TestSettingsPutKeepsSecretWhenEmpty(t *testing.T) {
	s := newTestSettings(t, config.Config{}, nil)
	ctx := context.Background()

	first := domain.DefaultSettings()
	first.Auth.OIDC.Enabled = true
	first.Auth.OIDC.IssuerURL = "https://issuer.example"
	first.Auth.OIDC.ClientID = "client-1"
	first.Auth.OIDC.ClientSecret = "top-secret"
	out, err := s.Put(ctx, first)
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	if out.Auth.OIDC.ClientSecret != "top-secret" {
		t.Fatalf("expected secret to be stored, got %q", out.Auth.OIDC.ClientSecret)
	}

	second := out
	second.Auth.OIDC.ClientSecret = "" // "keep existing"
	second.Auth.OIDC.ClientID = "client-2"
	out2, err := s.Put(ctx, second)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if out2.Auth.OIDC.ClientSecret != "top-secret" {
		t.Fatalf("expected stored secret to be kept, got %q", out2.Auth.OIDC.ClientSecret)
	}
	if out2.Auth.OIDC.ClientID != "client-2" {
		t.Fatalf("expected client id to change, got %q", out2.Auth.OIDC.ClientID)
	}

	stored, err := s.Get(ctx)
	if err != nil || stored.Auth.OIDC.ClientSecret != "top-secret" {
		t.Fatalf("secret not persisted correctly: %+v err=%v", stored, err)
	}
}

func TestSettingsRedacted(t *testing.T) {
	s := newTestSettings(t, config.Config{BaseURL: "https://vsui.example"}, nil)
	in := domain.DefaultSettings()
	in.Auth.OIDC.ClientSecret = "shh"
	out := s.Redacted(in)
	if out.Auth.OIDC.ClientSecret != "" {
		t.Fatalf("expected secret to be blanked, got %q", out.Auth.OIDC.ClientSecret)
	}
	want := "https://vsui.example/api/v1/auth/oidc/callback"
	if out.Auth.OIDC.RedirectURI != want {
		t.Fatalf("redirect uri = %q, want %q", out.Auth.OIDC.RedirectURI, want)
	}
}

func TestSettingsPutInvalidatesOIDCCache(t *testing.T) {
	calls := 0
	s := newTestSettings(t, config.Config{}, func() { calls++ })
	ctx := context.Background()
	in := domain.DefaultSettings()
	if _, err := s.Put(ctx, in); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected onChange to be called once, got %d", calls)
	}
}
