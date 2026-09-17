// Package notify pushes alerts (instance crashed, jobs failed, updates
// available, low disk, player join/leave) to operator-configured channels —
// Discord, Slack, ntfy, Telegram, a generic webhook, and email — and logs
// every delivery attempt (F-1.1, docs/ARCHITECTURE.md).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Message is one alert to deliver to a channel.
type Message struct {
	Kind       string
	Title      string
	Body       string
	InstanceID string
	At         time.Time
}

// Sender delivers one Message to one channel.
type Sender interface {
	Send(ctx context.Context, ch domain.NotifyChannel, m Message) error
}

// httpClient is shared by every HTTP-based sender.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// senderFor returns the Sender implementation for a channel type, or nil for
// an unknown type.
func senderFor(t domain.NotifyChannelType) Sender {
	switch t {
	case domain.NotifyChannelDiscord:
		return discordSender{}
	case domain.NotifyChannelSlack:
		return slackSender{}
	case domain.NotifyChannelNtfy:
		return ntfySender{}
	case domain.NotifyChannelTelegram:
		return telegramSender{}
	case domain.NotifyChannelWebhook:
		return webhookSender{}
	case domain.NotifyChannelEmail:
		return emailSender{}
	default:
		return nil
	}
}

// postJSON POSTs payload as JSON to dest, setting Authorization: Bearer
// <bearer> when bearer is non-empty.
func postJSON(ctx context.Context, kind, dest string, payload any, bearer string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%s: encode payload: %w", kind, err)
	}
	return doPost(ctx, kind, dest, "application/json", body, bearer, nil)
}

// doPost is the shared POST used by every HTTP-based sender. Non-2xx
// responses become an error naming kind and the status code; extraHeaders
// (may be nil) are set after Content-Type/Authorization so a caller can
// override them if ever needed. Every returned error is sanitized
// (sanitizeTransportErr) since callers log it and persist it verbatim in the
// notification_log table, and for discord/slack/telegram dest IS the bearer
// credential.
func doPost(ctx context.Context, kind, dest, contentType string, body []byte, bearer string, extraHeaders map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dest, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s: build request: %w", kind, sanitizeTransportErr(err))
	}
	req.Header.Set("Content-Type", contentType)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", kind, sanitizeTransportErr(err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: HTTP %d", kind, resp.StatusCode)
	}
	return nil
}

// sanitizeTransportErr strips the request URL that net/http embeds in a
// *url.Error's Error() string (format: `Op "URL": cause`), returning just
// the wrapped cause. Without this, a connection-level failure (DNS, refused,
// TLS, timeout, or a malformed URL) would leak the destination verbatim —
// which for discord/slack/telegram IS the bearer credential — into a
// returned error that callers log and store in the notification_log table.
func sanitizeTransportErr(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) && uerr.Err != nil {
		return uerr.Err
	}
	return err
}
