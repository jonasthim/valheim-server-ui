package notify

import (
	"context"
	"fmt"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// slackSender posts to a Slack incoming webhook URL.
type slackSender struct{}

func (slackSender) Send(ctx context.Context, ch domain.NotifyChannel, m Message) error {
	payload := map[string]string{"text": fmt.Sprintf("*%s*\n%s", m.Title, m.Body)}
	return postJSON(ctx, string(domain.NotifyChannelSlack), ch.URL, payload, "")
}
