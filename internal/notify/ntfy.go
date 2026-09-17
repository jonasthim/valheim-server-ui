package notify

import (
	"context"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// ntfySender posts the message body as plain text to a ntfy topic URL
// (https://ntfy.sh/<topic> or a self-hosted server).
type ntfySender struct{}

func (ntfySender) Send(ctx context.Context, ch domain.NotifyChannel, m Message) error {
	priority := "default"
	if m.Kind == domain.AlertCrashed || m.Kind == domain.AlertDown {
		priority = "high"
	}
	headers := map[string]string{"Title": m.Title, "Priority": priority}
	return doPost(ctx, string(domain.NotifyChannelNtfy), ch.URL, "text/plain; charset=utf-8", []byte(m.Body), ch.Secret, headers)
}
