package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"net/url"
	"strings"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// emailSender sends a plain-text email over SMTP. ch.URL has the shape
// "smtp://[user@]host:port/recipient@example.com" (see
// internal/auth/settings.go validateEmailURL); ch.Secret is the SMTP
// password, used only when the URL carries a user.
type emailSender struct{}

func (emailSender) Send(ctx context.Context, ch domain.NotifyChannel, m Message) error {
	u, err := url.Parse(ch.URL)
	if err != nil {
		return fmt.Errorf("email: parse url: %w", err)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "25"
	}
	addr := net.JoinHostPort(host, port)
	to := strings.TrimPrefix(u.Path, "/")
	if to == "" {
		return fmt.Errorf("email: no recipient in url")
	}
	var user string
	if u.User != nil {
		user = u.User.Username()
	}

	// smtp.Dial has no deadline; dial through ctx so a dead SMTP host cannot
	// hold the delivery goroutine past the service's send timeout.
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("email: dial %s: %w", addr, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("email: smtp handshake: %w", err)
	}
	defer func() { _ = c.Close() }()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("email: starttls: %w", err)
		}
	}
	if user != "" {
		if err := c.Auth(smtp.PlainAuth("", user, ch.Secret, host)); err != nil {
			return fmt.Errorf("email: auth: %w", err)
		}
	}
	from := user
	if from == "" {
		from = "valheim-ui@" + host
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("email: mail from: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("email: rcpt to: %w", err)
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("email: data: %w", err)
	}
	msg := fmt.Sprintf("Subject: %s\r\n\r\n%s\r\n", m.Title, m.Body)
	if _, err := wc.Write([]byte(msg)); err != nil {
		_ = wc.Close()
		return fmt.Errorf("email: write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("email: close: %w", err)
	}
	return c.Quit()
}
