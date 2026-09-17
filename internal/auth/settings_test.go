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

func TestSettingsPutNotifications_UnknownTypeRejected(t *testing.T) {
	s := newTestSettings(t, config.Config{}, nil)
	in := domain.DefaultSettings()
	in.Notifications.Channels = []domain.NotifyChannel{{
		Type: "carrier-pigeon", Name: "Pigeon", URL: "https://example.com/hook", Events: []string{domain.AlertCrashed},
	}}
	_, err := s.Put(context.Background(), in)
	de := domain.AsError(err)
	if de.Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed for an unknown channel type, got %v (%v)", de.Code, err)
	}
}

func TestSettingsPutNotifications_DiscordForeignHostRejected(t *testing.T) {
	s := newTestSettings(t, config.Config{}, nil)
	in := domain.DefaultSettings()
	in.Notifications.Channels = []domain.NotifyChannel{{
		Type: domain.NotifyChannelDiscord, Name: "Alerts", URL: "https://evil.example.com/webhook", Events: []string{domain.AlertCrashed},
	}}
	_, err := s.Put(context.Background(), in)
	de := domain.AsError(err)
	if de.Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed for a discord url on a foreign host, got %v (%v)", de.Code, err)
	}

	// The real discord.com host (any path) is accepted.
	in.Notifications.Channels[0].URL = "https://discord.com/api/webhooks/1/abc"
	if _, err := s.Put(context.Background(), in); err != nil {
		t.Fatalf("expected a discord.com webhook url to be accepted, got %v", err)
	}
}

func TestSettingsPutNotifications_EventsAndInstancesValidated(t *testing.T) {
	s := newTestSettings(t, config.Config{}, nil)
	ctx := context.Background()

	badEvent := domain.DefaultSettings()
	badEvent.Notifications.Channels = []domain.NotifyChannel{{
		Type: domain.NotifyChannelWebhook, Name: "Hook", URL: "https://example.com/hook", Events: []string{"not-a-real-alert"},
	}}
	if _, err := s.Put(ctx, badEvent); domain.AsError(err).Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed for an unknown alert kind, got %v", err)
	}

	badInstance := domain.DefaultSettings()
	badInstance.Notifications.Channels = []domain.NotifyChannel{{
		Type: domain.NotifyChannelWebhook, Name: "Hook", URL: "https://example.com/hook",
		Events: []string{domain.AlertCrashed}, Instances: []string{"Not A Slug!"},
	}}
	if _, err := s.Put(ctx, badInstance); domain.AsError(err).Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed for an invalid instance slug, got %v", err)
	}

	badPercent := domain.DefaultSettings()
	badPercent.Notifications.DiskLowPercent = 101
	if _, err := s.Put(ctx, badPercent); domain.AsError(err).Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed for disk_low_percent > 100, got %v", err)
	}
}

func TestSettingsPutNotifications_AssignsIDAndKeepsSecretWhenBlank(t *testing.T) {
	s := newTestSettings(t, config.Config{}, nil)
	ctx := context.Background()

	first := domain.DefaultSettings()
	first.Notifications.Channels = []domain.NotifyChannel{{
		Type: domain.NotifyChannelDiscord, Name: "Alerts", URL: "https://discord.com/api/webhooks/1/abc",
		Secret: "s3cr3t", Enabled: true, Events: []string{domain.AlertCrashed},
	}}
	out, err := s.Put(ctx, first)
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	if len(out.Notifications.Channels) != 1 || out.Notifications.Channels[0].ID == "" {
		t.Fatalf("expected a channel with a freshly assigned id, got %+v", out.Notifications.Channels)
	}
	if out.Notifications.Channels[0].Secret != "s3cr3t" {
		t.Fatalf("expected the secret to be stored, got %q", out.Notifications.Channels[0].Secret)
	}
	id := out.Notifications.Channels[0].ID

	second := domain.DefaultSettings()
	second.Notifications.Channels = []domain.NotifyChannel{{
		ID: id, Type: domain.NotifyChannelDiscord, Name: "Alerts renamed", URL: "https://discord.com/api/webhooks/1/abc",
		Secret: "", Enabled: true, Events: []string{domain.AlertCrashed},
	}}
	out2, err := s.Put(ctx, second)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if out2.Notifications.Channels[0].Secret != "s3cr3t" {
		t.Fatalf("expected the stored secret to be kept, got %q", out2.Notifications.Channels[0].Secret)
	}
	if out2.Notifications.Channels[0].ID != id {
		t.Fatalf("expected the id to be preserved, got %q want %q", out2.Notifications.Channels[0].ID, id)
	}
	if out2.Notifications.Channels[0].Name != "Alerts renamed" {
		t.Fatalf("expected the name to change, got %q", out2.Notifications.Channels[0].Name)
	}

	stored, err := s.Get(ctx)
	if err != nil || stored.Notifications.Channels[0].Secret != "s3cr3t" {
		t.Fatalf("secret not persisted correctly: %+v err=%v", stored, err)
	}
}

func TestSettingsRedactedBlanksNotificationSecrets(t *testing.T) {
	s := newTestSettings(t, config.Config{}, nil)
	in := domain.DefaultSettings()
	in.Notifications.Channels = []domain.NotifyChannel{{
		ID: "c1", Type: domain.NotifyChannelWebhook, Name: "Hook", URL: "https://example.com/hook",
		Secret: "shh", Events: []string{domain.AlertCrashed},
	}}
	out := s.Redacted(in)
	if out.Notifications.Channels[0].Secret != "" {
		t.Fatalf("expected the notification channel secret to be blanked, got %q", out.Notifications.Channels[0].Secret)
	}
	// The original passed to Redacted must not be mutated (mirrors the OIDC case).
	if in.Notifications.Channels[0].Secret != "shh" {
		t.Fatalf("Redacted must not mutate its input, got %q", in.Notifications.Channels[0].Secret)
	}
}

// TestSettingsRedactedBlanksURLForTypesThatEmbedASecret covers the security
// review finding: discord/slack/telegram webhook URLs carry the bearer
// credential in the URL itself, so Redacted must blank URL too, not just
// Secret; ntfy (an address, not a credential) must keep its URL visible.
func TestSettingsRedactedBlanksURLForTypesThatEmbedASecret(t *testing.T) {
	s := newTestSettings(t, config.Config{}, nil)
	in := domain.DefaultSettings()
	in.Notifications.Channels = []domain.NotifyChannel{
		{ID: "c1", Type: domain.NotifyChannelDiscord, Name: "Discord", URL: "https://discord.com/api/webhooks/1/topsecret", Events: []string{domain.AlertCrashed}},
		{ID: "c2", Type: domain.NotifyChannelNtfy, Name: "Ntfy", URL: "https://ntfy.sh/my-topic", Events: []string{domain.AlertCrashed}},
	}
	out := s.Redacted(in)
	if out.Notifications.Channels[0].URL != "" {
		t.Fatalf("expected the discord channel's url to be blanked, got %q", out.Notifications.Channels[0].URL)
	}
	if out.Notifications.Channels[1].URL != "https://ntfy.sh/my-topic" {
		t.Fatalf("expected the ntfy channel's url to stay visible, got %q", out.Notifications.Channels[1].URL)
	}
	// The original passed to Redacted must not be mutated.
	if in.Notifications.Channels[0].URL == "" {
		t.Fatalf("Redacted must not mutate its input")
	}
}

// TestSettingsPutNotifications_BlankURLKeepsStored covers the security
// review finding: since discord/slack/telegram URLs are never returned by a
// read, the UI must be able to re-save a channel (rename it, change its
// events) without re-pasting the URL, exactly like the Secret rule.
func TestSettingsPutNotifications_BlankURLKeepsStored(t *testing.T) {
	s := newTestSettings(t, config.Config{}, nil)
	ctx := context.Background()

	first := domain.DefaultSettings()
	first.Notifications.Channels = []domain.NotifyChannel{{
		Type: domain.NotifyChannelDiscord, Name: "Alerts", URL: "https://discord.com/api/webhooks/1/abc",
		Secret: "", Enabled: true, Events: []string{domain.AlertCrashed},
	}}
	out, err := s.Put(ctx, first)
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	id := out.Notifications.Channels[0].ID
	storedURL := out.Notifications.Channels[0].URL
	if storedURL == "" {
		t.Fatalf("expected the url to be stored, got %+v", out.Notifications.Channels[0])
	}

	second := domain.DefaultSettings()
	second.Notifications.Channels = []domain.NotifyChannel{{
		ID: id, Type: domain.NotifyChannelDiscord, Name: "Alerts renamed", URL: "",
		Enabled: true, Events: []string{domain.AlertCrashed},
	}}
	out2, err := s.Put(ctx, second)
	if err != nil {
		t.Fatalf("second Put (blank url): %v", err)
	}
	if out2.Notifications.Channels[0].URL != storedURL {
		t.Fatalf("expected the stored url to be kept, got %q want %q", out2.Notifications.Channels[0].URL, storedURL)
	}
	if out2.Notifications.Channels[0].Name != "Alerts renamed" {
		t.Fatalf("expected the name to change, got %q", out2.Notifications.Channels[0].Name)
	}
}

// TestSettingsPutNotifications_NewChannelWithBlankURLRejected covers the
// security review finding: a channel with no id (so nothing stored to fall
// back to) and a blank URL must still fail validation, not silently save
// with an empty destination.
func TestSettingsPutNotifications_NewChannelWithBlankURLRejected(t *testing.T) {
	s := newTestSettings(t, config.Config{}, nil)
	in := domain.DefaultSettings()
	in.Notifications.Channels = []domain.NotifyChannel{{
		Type: domain.NotifyChannelWebhook, Name: "Hook", URL: "", Events: []string{domain.AlertCrashed},
	}}
	_, err := s.Put(context.Background(), in)
	if domain.AsError(err).Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed for a brand-new channel with a blank url, got %v", err)
	}
}
