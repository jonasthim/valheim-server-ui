package notify

import (
	"context"
	"fmt"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// discordSender posts to a Discord incoming webhook URL.
type discordSender struct{}

func (discordSender) Send(ctx context.Context, ch domain.NotifyChannel, m Message) error {
	payload := map[string]string{"content": fmt.Sprintf("**%s**\n%s", m.Title, m.Body)}
	return postJSON(ctx, string(domain.NotifyChannelDiscord), ch.URL, payload, "")
}
