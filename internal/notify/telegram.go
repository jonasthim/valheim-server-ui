package notify

import (
	"context"
	"fmt"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// telegramSender posts to the Telegram Bot API sendMessage endpoint. ch.URL
// is the full "https://api.telegram.org/bot<token>/sendMessage" URL; ch.Secret
// holds the destination chat id.
type telegramSender struct{}

func (telegramSender) Send(ctx context.Context, ch domain.NotifyChannel, m Message) error {
	payload := map[string]string{"chat_id": ch.Secret, "text": fmt.Sprintf("%s\n%s", m.Title, m.Body)}
	return postJSON(ctx, string(domain.NotifyChannelTelegram), ch.URL, payload, "")
}
