package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func newRecorder(t *testing.T) *Recorder {
	t.Helper()
	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	return New(db.NewAuditRepo(sqldb), nil)
}

func TestRecorderReadsUserAndIPFromContext(t *testing.T) {
	r := newRecorder(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/main/start", nil)
	req.RemoteAddr = "203.0.113.5:54321"
	usr := &domain.User{ID: 7, Username: "alice", Role: domain.RoleOperator}
	req = req.WithContext(api.WithUser(req.Context(), usr))

	r.Record(req, "instance.start", "main", "main", map[string]any{"foo": "bar"})

	entries, err := r.List(context.Background(), "", "", 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Username != "alice" || e.UserID == nil || *e.UserID != 7 {
		t.Fatalf("user not recorded: %+v", e)
	}
	if e.Action != "instance.start" || e.InstanceID != "main" || e.Target != "main" {
		t.Fatalf("action/target not recorded: %+v", e)
	}
	if e.IP != "203.0.113.5" {
		t.Fatalf("expected IP without port, got %q", e.IP)
	}
	if e.Details["foo"] != "bar" {
		t.Fatalf("details not recorded: %+v", e.Details)
	}
}

func TestRecorderAnonymousRequest(t *testing.T) {
	r := newRecorder(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup", nil)
	req.RemoteAddr = "10.0.0.1:1111"

	r.Record(req, "auth.setup", "", "admin", nil)

	entries, err := r.List(context.Background(), "", "", 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].UserID != nil {
		t.Fatalf("expected anonymous entry with nil user id, got %+v", entries)
	}
}
