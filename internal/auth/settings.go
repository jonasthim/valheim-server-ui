package auth

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

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

	fields = append(fields, validateNotifications(&out.Notifications, current.Notifications)...)
	fields = append(fields, validateBackupTargets(&out.Backups.Targets, current.Backups.Targets)...)

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

// Redacted blanks the OIDC client secret, every notification channel secret,
// and the URL of channel types whose URL itself carries a bearer credential
// (discord/slack webhook paths, a telegram bot token), and fills in the
// computed redirect URI, per the OpenAPI contract for API responses.
func (s *Settings) Redacted(in domain.Settings) domain.Settings {
	out := in
	out.Auth.OIDC.ClientSecret = ""
	out.Auth.OIDC.RedirectURI = s.cfg.BaseURL + oidcCallbackPath
	if len(out.Notifications.Channels) > 0 {
		chans := make([]domain.NotifyChannel, len(out.Notifications.Channels))
		copy(chans, out.Notifications.Channels)
		for i := range chans {
			chans[i].Secret = ""
			if channelURLEmbedsSecret(chans[i].Type) {
				chans[i].URL = ""
			}
		}
		out.Notifications.Channels = chans
	}
	return out
}

// channelURLEmbedsSecret reports whether a channel type's URL itself carries
// a bearer credential (a Discord/Slack incoming-webhook path, a Telegram bot
// token) rather than just an endpoint address, so it must be redacted like
// Secret instead of being shown back to the client. ntfy/webhook/email URLs
// are plain addresses (their credential, if any, lives in Secret) and stay
// visible.
func channelURLEmbedsSecret(t domain.NotifyChannelType) bool {
	switch t {
	case domain.NotifyChannelDiscord, domain.NotifyChannelSlack, domain.NotifyChannelTelegram:
		return true
	default:
		return false
	}
}

// validNotifyChannelType reports whether t is one of the known channel types.
func validNotifyChannelType(t domain.NotifyChannelType) bool {
	for _, known := range domain.NotifyChannelTypes {
		if t == known {
			return true
		}
	}
	return false
}

// validAlertKind reports whether k is one of domain.AllAlertKinds.
func validAlertKind(k string) bool {
	for _, known := range domain.AllAlertKinds {
		if k == known {
			return true
		}
	}
	return false
}

// validateNotifications merges (id assignment, "blank secret keeps stored")
// and validates in place, returning field errors for anything invalid.
// current is the previously stored notification settings, used to look up a
// channel's stored secret by id (same rule as auth.oidc.client_secret).
func validateNotifications(out *domain.NotifySettings, current domain.NotifySettings) []domain.FieldError {
	currentByID := make(map[string]domain.NotifyChannel, len(current.Channels))
	for _, ch := range current.Channels {
		currentByID[ch.ID] = ch
	}

	var fields []domain.FieldError
	for i := range out.Channels {
		ch := &out.Channels[i]
		prefix := fmt.Sprintf("notifications.channels.%d", i)

		typeChanged := false
		if ch.ID == "" {
			ch.ID = uuid.NewString()
		} else if prev, ok := currentByID[ch.ID]; ok {
			typeChanged = ch.Type != prev.Type
			// A blank submitted Secret or URL keeps the stored value (same
			// rule as auth.oidc.client_secret): Secret is always write-only,
			// and discord/slack/telegram also blank URL on read since it
			// embeds a bearer credential (see channelURLEmbedsSecret).
			// Never across a type change: the stored values belong to the
			// old provider, and carrying a credential-embedding URL over to
			// a type whose URL is returned in the clear would disclose it.
			if !typeChanged {
				if ch.Secret == "" {
					ch.Secret = prev.Secret
				}
				if ch.URL == "" {
					ch.URL = prev.URL
				}
			}
		}

		if !validNotifyChannelType(ch.Type) {
			fields = append(fields, domain.FieldError{Field: prefix + ".type", Message: "unknown channel type"})
			continue // URL/host validation below assumes a known type
		}
		if l := len(ch.Name); l < 1 || l > 64 {
			fields = append(fields, domain.FieldError{Field: prefix + ".name", Message: "must be 1-64 characters"})
		}
		if typeChanged && ch.URL == "" {
			fields = append(fields, domain.FieldError{Field: prefix + ".url", Message: "enter the destination again when changing the channel type"})
		} else if err := validateChannelURL(ch); err != "" {
			fields = append(fields, domain.FieldError{Field: prefix + ".url", Message: err})
		}
		for _, ev := range ch.Events {
			if !validAlertKind(ev) {
				fields = append(fields, domain.FieldError{Field: prefix + ".events", Message: "unknown alert kind: " + ev})
				break
			}
		}
		for _, inst := range ch.Instances {
			if !domain.InstanceIDPattern.MatchString(inst) {
				fields = append(fields, domain.FieldError{Field: prefix + ".instances", Message: "invalid instance id: " + inst})
				break
			}
		}
	}

	if out.DiskLowPercent < 0 || out.DiskLowPercent > 100 {
		fields = append(fields, domain.FieldError{Field: "notifications.disk_low_percent", Message: "must be 0-100"})
	}
	return fields
}

// validateChannelURL checks ch.URL against the shape and host allowlist for
// ch.Type, returning an empty string when valid. Discord/Slack/Telegram must
// point at the real provider host over https; ntfy/webhook accept any host
// and allow http (self-hosted ntfy); email uses a distinct smtp:// shape.
func validateChannelURL(ch *domain.NotifyChannel) string {
	if ch.Type == domain.NotifyChannelEmail {
		return validateEmailURL(ch.URL)
	}

	u, err := url.Parse(ch.URL)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return "must be an absolute URL"
	}
	switch u.Scheme {
	case "https":
	case "http":
		if ch.Type != domain.NotifyChannelNtfy && ch.Type != domain.NotifyChannelWebhook {
			return "must use https://"
		}
	default:
		return "must be an http(s) URL"
	}

	host := strings.ToLower(u.Hostname())
	switch ch.Type {
	case domain.NotifyChannelDiscord:
		if host != "discord.com" && host != "discordapp.com" {
			return "discord webhook URLs must point at discord.com or discordapp.com"
		}
	case domain.NotifyChannelSlack:
		if host != "hooks.slack.com" {
			return "slack webhook URLs must point at hooks.slack.com"
		}
	case domain.NotifyChannelTelegram:
		if host != "api.telegram.org" {
			return "telegram URLs must point at api.telegram.org"
		}
	case domain.NotifyChannelNtfy, domain.NotifyChannelWebhook:
		// Any host, including LAN and loopback addresses over plain http:
		// a self-hosted ntfy or a Home Assistant webhook on the local
		// network is the primary use case for a homelab manager. Only
		// admins can configure channels, and the request is a POST with
		// fixed alert text, so this is not an SSRF surface worth closing at
		// the cost of the intended deployments.
	}
	return ""
}

// validateEmailURL checks the "smtp://[user@]host:port/to@example.com" shape
// used to configure the email channel: userinfo carries the SMTP username
// (optional, for servers that allow anonymous relay), host:port is the
// server address, and the path carries the single recipient address.
func validateEmailURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "smtp" || u.Hostname() == "" {
		return "must be smtp://[user@]host:port/recipient@example.com"
	}
	to := strings.TrimPrefix(u.Path, "/")
	if to == "" || !strings.Contains(to, "@") {
		return "path must be the recipient address, e.g. /ops@example.com"
	}
	return ""
}

// validateBackupTargets assigns ids (F-1.4) and validates *out in place,
// mirroring validateNotifications's id-assignment + validation style:
// unlike a channel's secret/url, a target's path/remote are not write-only,
// so there is no "blank keeps stored" rule here -- every field is always
// resubmitted in full. current is unused beyond the id lookup that keeps a
// resubmitted target's id stable across saves.
func validateBackupTargets(out *[]domain.BackupTarget, current []domain.BackupTarget) []domain.FieldError {
	currentByID := make(map[string]domain.BackupTarget, len(current))
	for _, t := range current {
		currentByID[t.ID] = t
	}

	var fields []domain.FieldError
	for i := range *out {
		t := &(*out)[i]
		prefix := fmt.Sprintf("backups.targets.%d", i)

		if t.ID == "" {
			t.ID = uuid.NewString()
		}

		switch t.Type {
		case domain.BackupTargetLocal, domain.BackupTargetRclone:
		default:
			fields = append(fields, domain.FieldError{Field: prefix + ".type", Message: "must be local or rclone"})
			continue // path/remote checks below assume a known type
		}
		if l := len(t.Name); l < 1 || l > 64 {
			fields = append(fields, domain.FieldError{Field: prefix + ".name", Message: "must be 1-64 characters"})
		}
		switch t.Type {
		case domain.BackupTargetLocal:
			if !filepath.IsAbs(t.Path) {
				fields = append(fields, domain.FieldError{Field: prefix + ".path", Message: "must be an absolute path"})
			}
		case domain.BackupTargetRclone:
			if t.Remote == "" || !strings.Contains(t.Remote, ":") {
				fields = append(fields, domain.FieldError{Field: prefix + ".remote", Message: `must be a remote in the form "name:path"`})
			}
		}
		if t.KeepLast < 0 || t.KeepLast > 1000 {
			fields = append(fields, domain.FieldError{Field: prefix + ".keep_last", Message: "must be 0-1000"})
		}
	}
	return fields
}
