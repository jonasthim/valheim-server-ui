package notify

import (
	"context"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// webhookSender posts the Message as JSON to a generic URL, with an optional
// bearer token.
type webhookSender struct{}

func (webhookSender) Send(ctx context.Context, ch domain.NotifyChannel, m Message) error {
	return postJSON(ctx, string(domain.NotifyChannelWebhook), ch.URL, m, ch.Secret)
}
