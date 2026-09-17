package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// notifyFakeService implements NotifyService for handler tests.
type notifyFakeService struct {
	testErr error

	lastTestID string

	logEntries    []domain.NotificationLogEntry
	logErr        error
	lastLogLimit  int
	lastLogBefore *time.Time
}

func (f *notifyFakeService) Test(_ context.Context, channelID string) error {
	f.lastTestID = channelID
	return f.testErr
}

func (f *notifyFakeService) Log(_ context.Context, limit int, before *time.Time) ([]domain.NotificationLogEntry, error) {
	f.lastLogLimit = limit
	f.lastLogBefore = before
	if f.logErr != nil {
		return nil, f.logErr
	}
	return f.logEntries, nil
}

// newNotifyTestDeps wires a Deps with the notifyFakeService and reuses
// systemFakeAuth/systemFakeAuditor (defined in system_handlers_test.go,
// same package) to inject a fixed-role user and capture audit calls.
func newNotifyTestDeps(t *testing.T, role domain.Role) (*Deps, *notifyFakeService, *systemFakeAuditor) {
	t.Helper()
	svc := &notifyFakeService{}
	audit := &systemFakeAuditor{}
	d := &Deps{
		Log:    slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 4})),
		Notify: svc,
		Audit:  audit,
		Auth:   systemFakeAuth{user: &domain.User{ID: 1, Username: "tester", Role: role}},
	}
	return d, svc, audit
}

func TestNotifyTestHandler_CallsServiceAndAudits(t *testing.T) {
	d, svc, audit := newNotifyTestDeps(t, domain.RoleAdmin)
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/notifications/c1/test", nil)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.lastTestID != "c1" {
		t.Fatalf("expected the service to be called with channel id c1, got %q", svc.lastTestID)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("expected {\"ok\":true}, got %v", body)
	}
	if len(audit.calls) != 1 || audit.calls[0].action != "notifications.test" || audit.calls[0].target != "c1" {
		t.Fatalf("expected one notifications.test audit record for c1, got %+v", audit.calls)
	}
}

func TestNotifyTestHandler_ServiceErrorPropagates(t *testing.T) {
	d, svc, _ := newNotifyTestDeps(t, domain.RoleAdmin)
	svc.testErr = domain.E(domain.CodeValidationFailed, "unsupported channel type")
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/notifications/c1/test", nil)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotifyTestHandler_RequiresAdmin(t *testing.T) {
	d, _, _ := newNotifyTestDeps(t, domain.RoleOperator)
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/notifications/c1/test", nil)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for an operator, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListNotificationsHandler_ReturnsEntriesAndPassesLimit(t *testing.T) {
	d, svc, _ := newNotifyTestDeps(t, domain.RoleAdmin)
	svc.logEntries = []domain.NotificationLogEntry{{ID: 1, ChannelID: "c1", Kind: "crashed", OK: true}}
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?limit=5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Entries []domain.NotificationLogEntry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Entries) != 1 || body.Entries[0].ChannelID != "c1" {
		t.Fatalf("unexpected entries: %+v", body.Entries)
	}
	if svc.lastLogLimit != 5 {
		t.Fatalf("expected limit 5 to be passed through, got %d", svc.lastLogLimit)
	}
}

func TestListNotificationsHandler_BeforeCursorParsed(t *testing.T) {
	d, svc, _ := newNotifyTestDeps(t, domain.RoleAdmin)
	router := NewRouter(d, nil)

	before := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?before="+before.Format(time.RFC3339), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.lastLogBefore == nil || !svc.lastLogBefore.Equal(before) {
		t.Fatalf("expected the before cursor to be parsed through, got %v", svc.lastLogBefore)
	}
}

func TestListNotificationsHandler_InvalidLimit(t *testing.T) {
	d, _, _ := newNotifyTestDeps(t, domain.RoleAdmin)
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?limit=nope", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for a non-numeric limit, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListNotificationsHandler_RequiresAdmin(t *testing.T) {
	d, _, _ := newNotifyTestDeps(t, domain.RoleViewer)
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a viewer, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotifyRoutes_ServiceNotConfigured(t *testing.T) {
	d, _, _ := newNotifyTestDeps(t, domain.RoleAdmin)
	d.Notify = nil
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when the notify service is not configured, got %d: %s", rec.Code, rec.Body.String())
	}
}
