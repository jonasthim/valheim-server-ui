package api

import (
	"reflect"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestAuditDiff(t *testing.T) {
	before := domain.DefaultInstanceConfig()
	before.Name = "Berra"
	before.Password = "old-secret"
	before.Modifiers.Portals = "hard"
	before.SetKeys = []string{"nomap"}

	after := before
	after.Password = "new-secret"
	after.Modifiers.Portals = ""
	after.Modifiers.Combat = "easy"
	after.SetKeys = []string{"nomap", "fire"}

	got := auditDiff(map[string]any{"config": before}, map[string]any{"config": after})
	want := []auditChange{
		{Path: "config.modifiers.combat", To: "easy"},
		{Path: "config.modifiers.portals", From: "hard"},
		{Path: "config.password", From: maskedValue, To: maskedValue},
		{Path: "config.setkeys", From: []any{"nomap"}, To: []any{"nomap", "fire"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diff mismatch:\n got %#v\nwant %#v", got, want)
	}
}

func TestAuditDiffNoChangeAndScalars(t *testing.T) {
	if got := auditDiff(map[string]any{"a": 1, "b": "x"}, map[string]any{"a": 1, "b": "x"}); len(got) != 0 {
		t.Fatalf("expected no changes, got %#v", got)
	}
	got := auditDiff(map[string]any{"autostart": false, "name": "A"}, map[string]any{"autostart": true, "name": "A"})
	if len(got) != 1 || got[0].Path != "autostart" || got[0].From != false || got[0].To != true {
		t.Fatalf("unexpected diff %#v", got)
	}
	if got := auditDiff(nil, map[string]any{"x": 1}); len(got) != 1 || got[0].Path != "x" {
		t.Fatalf("nil before should report the new keys, got %#v", got)
	}
}
