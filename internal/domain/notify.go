package domain

import "time"

// NotifyChannelType is the kind of outbound notification channel (F-1.1).
type NotifyChannelType string

const (
	NotifyChannelDiscord  NotifyChannelType = "discord"
	NotifyChannelSlack    NotifyChannelType = "slack"
	NotifyChannelNtfy     NotifyChannelType = "ntfy"
	NotifyChannelTelegram NotifyChannelType = "telegram"
	NotifyChannelWebhook  NotifyChannelType = "webhook"
	NotifyChannelEmail    NotifyChannelType = "email"
)

// NotifyChannelTypes lists every known channel type, for validation.
var NotifyChannelTypes = []NotifyChannelType{
	NotifyChannelDiscord, NotifyChannelSlack, NotifyChannelNtfy,
	NotifyChannelTelegram, NotifyChannelWebhook, NotifyChannelEmail,
}

// Alert kinds: the events an operator can subscribe a channel to.
const (
	AlertCrashed     = "crashed"
	AlertDown        = "down"
	AlertJobFailed   = "job_failed"
	AlertGameUpdate  = "game_update"
	AlertAppUpdate   = "app_update"
	AlertDiskLow     = "disk_low"
	AlertPlayerJoin  = "player_join"
	AlertPlayerLeave = "player_leave"
)

// AllAlertKinds lists every known alert kind, for validation and the UI.
var AllAlertKinds = []string{
	AlertCrashed, AlertDown, AlertJobFailed, AlertGameUpdate,
	AlertAppUpdate, AlertDiskLow, AlertPlayerJoin, AlertPlayerLeave,
}

// NotifyChannel is one configured outbound notification destination.
// Secret is write-only: PUT /settings stores it, but it is never returned by
// a read (Settings.Redacted blanks it, like auth.oidc.client_secret).
type NotifyChannel struct {
	ID        string            `json:"id"`
	Type      NotifyChannelType `json:"type"`
	Name      string            `json:"name"`
	Enabled   bool              `json:"enabled"`
	URL       string            `json:"url,omitempty"`
	Secret    string            `json:"secret,omitempty"`
	Events    []string          `json:"events"`
	Instances []string          `json:"instances"`
}

// NotifySettings is the notifications block of domain.Settings.
type NotifySettings struct {
	Channels []NotifyChannel `json:"channels"`
	// DiskLowPercent is the free-space threshold that raises AlertDiskLow.
	// 0 (the zero value) means "not configured": the service treats it as 10.
	DiskLowPercent int `json:"disk_low_percent"`
}

// NotificationLogEntry is one row of the notification_log table: a single
// delivery attempt, successful or not.
type NotificationLogEntry struct {
	ID         int64     `json:"id"`
	At         time.Time `json:"at"`
	ChannelID  string    `json:"channel_id"`
	Kind       string    `json:"kind"`
	InstanceID string    `json:"instance_id,omitempty"`
	OK         bool      `json:"ok"`
	Error      string    `json:"error,omitempty"`
}
