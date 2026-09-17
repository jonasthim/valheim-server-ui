package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestDiscordSender_PostsTitleAndBody(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := domain.NotifyChannel{Type: domain.NotifyChannelDiscord, URL: srv.URL}
	m := Message{Kind: domain.AlertCrashed, Title: "main crashed", Body: "exit status 1", At: time.Now()}
	if err := (discordSender{}).Send(context.Background(), ch, m); err != nil {
		t.Fatalf("send: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("decode body %q: %v", gotBody, err)
	}
	if !strings.Contains(payload["content"], m.Title) || !strings.Contains(payload["content"], m.Body) {
		t.Fatalf("expected content to contain title and body, got %q", payload["content"])
	}
}

func TestWebhookSender_SetsBearerHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := domain.NotifyChannel{Type: domain.NotifyChannelWebhook, URL: srv.URL, Secret: "tok123"}
	m := Message{Kind: domain.AlertDiskLow, Title: "low disk", Body: "5% free"}
	if err := (webhookSender{}).Send(context.Background(), ch, m); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotAuth != "Bearer tok123" {
		t.Fatalf("expected bearer header, got %q", gotAuth)
	}
}

func TestWebhookSender_NoSecretOmitsAuthHeader(t *testing.T) {
	var gotAuth string
	seen := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		seen = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := domain.NotifyChannel{Type: domain.NotifyChannelWebhook, URL: srv.URL}
	m := Message{Kind: domain.AlertDiskLow, Title: "t", Body: "b"}
	if err := (webhookSender{}).Send(context.Background(), ch, m); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !seen || gotAuth != "" {
		t.Fatalf("expected no Authorization header, got %q (seen=%v)", gotAuth, seen)
	}
}

func TestSender_HTTP500ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ch := domain.NotifyChannel{Type: domain.NotifyChannelDiscord, URL: srv.URL}
	m := Message{Kind: domain.AlertCrashed, Title: "t", Body: "b"}
	err := (discordSender{}).Send(context.Background(), ch, m)
	if err == nil {
		t.Fatal("expected an error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected the error to mention the status code, got %v", err)
	}
}

func TestSlackSender_PostsText(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := domain.NotifyChannel{Type: domain.NotifyChannelSlack, URL: srv.URL}
	m := Message{Kind: domain.AlertCrashed, Title: "main crashed", Body: "detail"}
	if err := (slackSender{}).Send(context.Background(), ch, m); err != nil {
		t.Fatalf("send: %v", err)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.Contains(payload["text"], "main crashed") || !strings.Contains(payload["text"], "detail") {
		t.Fatalf("expected text to contain title and body, got %q", payload["text"])
	}
}

func TestNtfySender_SetsTitleAndPriorityHeaders(t *testing.T) {
	var gotTitle, gotPriority, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTitle = r.Header.Get("Title")
		gotPriority = r.Header.Get("Priority")
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := domain.NotifyChannel{Type: domain.NotifyChannelNtfy, URL: srv.URL, Secret: "tok"}
	m := Message{Kind: domain.AlertCrashed, Title: "main crashed", Body: "boom"}
	if err := (ntfySender{}).Send(context.Background(), ch, m); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotTitle != "main crashed" {
		t.Fatalf("expected Title header, got %q", gotTitle)
	}
	if gotPriority != "high" {
		t.Fatalf("expected high priority for a crash alert, got %q", gotPriority)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("expected bearer auth from Secret, got %q", gotAuth)
	}
	if gotBody != "boom" {
		t.Fatalf("expected the body to be the plain message text, got %q", gotBody)
	}

	// A non-urgent kind gets default priority.
	m2 := Message{Kind: domain.AlertGameUpdate, Title: "t", Body: "b"}
	if err := (ntfySender{}).Send(context.Background(), ch, m2); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotPriority != "default" {
		t.Fatalf("expected default priority for a non-urgent alert, got %q", gotPriority)
	}
}

func TestTelegramSender_PostsChatIDFromSecret(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := domain.NotifyChannel{Type: domain.NotifyChannelTelegram, URL: srv.URL, Secret: "12345"}
	m := Message{Kind: domain.AlertCrashed, Title: "main crashed", Body: "boom"}
	if err := (telegramSender{}).Send(context.Background(), ch, m); err != nil {
		t.Fatalf("send: %v", err)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload["chat_id"] != "12345" {
		t.Fatalf("expected chat_id from Secret, got %q", payload["chat_id"])
	}
	if !strings.Contains(payload["text"], "main crashed") || !strings.Contains(payload["text"], "boom") {
		t.Fatalf("expected text to contain title and body, got %q", payload["text"])
	}
}

func TestSenderFor_KnownAndUnknownTypes(t *testing.T) {
	for _, typ := range domain.NotifyChannelTypes {
		if senderFor(typ) == nil {
			t.Errorf("expected a sender for type %q", typ)
		}
	}
	if senderFor("bogus") != nil {
		t.Fatal("expected no sender for an unknown type")
	}
}

// TestSender_TransportErrorDoesNotLeakURL covers the security review
// finding: net/http wraps a connection-level failure in a *url.Error whose
// Error() string embeds the full request URL. For discord/slack/telegram
// that URL IS the bearer credential, and this error is what gets logged and
// persisted verbatim in the notification_log table, so it must never
// surface here.
func TestSender_TransportErrorDoesNotLeakURL(t *testing.T) {
	const secretToken = "topsecretwebhooktoken12345"
	// Port 0 on a resolvable loopback address is refused immediately (no
	// server listening), giving a deterministic connection-level failure
	// without a real network dependency.
	ch := domain.NotifyChannel{Type: domain.NotifyChannelDiscord, URL: "http://127.0.0.1:1/api/webhooks/1/" + secretToken}
	m := Message{Kind: domain.AlertCrashed, Title: "t", Body: "b"}

	err := (discordSender{}).Send(context.Background(), ch, m)
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if strings.Contains(err.Error(), secretToken) {
		t.Fatalf("expected the webhook token not to appear in the error, got %q", err.Error())
	}
	if strings.Contains(err.Error(), ch.URL) {
		t.Fatalf("expected the url not to appear in the error at all, got %q", err.Error())
	}
}
