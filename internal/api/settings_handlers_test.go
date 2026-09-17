package api

import "testing"

// TestMaskNotifyChannelURLs covers the security review finding that a
// notification channel's url can itself carry a bearer credential
// (discord/slack webhook paths, a telegram bot token) and must never reach
// the audit log in the clear, the same way auditDiff already masks secret.
func TestMaskNotifyChannelURLs(t *testing.T) {
	in := []auditChange{
		{Path: "notifications.channels.0.url", From: "https://discord.com/api/webhooks/1/old", To: "https://discord.com/api/webhooks/1/new"},
		{Path: "notifications.channels.1.url", To: "https://ntfy.sh/my-topic"}, // a new channel: no From
		{Path: "notifications.channels.0.name", From: "Alerts", To: "Alerts renamed"},
		{Path: "notifications.disk_low_percent", From: float64(10), To: float64(20)},
	}
	got := maskNotifyChannelURLs(in)

	if got[0].From != maskedValue || got[0].To != maskedValue {
		t.Fatalf("expected channel 0 url to be masked, got %+v", got[0])
	}
	if got[1].From != nil {
		t.Fatalf("expected a nil From to stay nil (no value to mask), got %+v", got[1])
	}
	if got[1].To != maskedValue {
		t.Fatalf("expected channel 1 url to be masked, got %+v", got[1])
	}
	if got[2].From != "Alerts" || got[2].To != "Alerts renamed" {
		t.Fatalf("expected the name change to be left alone, got %+v", got[2])
	}
	if got[3].From != float64(10) || got[3].To != float64(20) {
		t.Fatalf("expected an unrelated notifications field to be left alone, got %+v", got[3])
	}
}
