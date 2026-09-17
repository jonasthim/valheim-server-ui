package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// agentFakeService implements AgentService for handler tests; only
// ChatHistory needs real behaviour for GET /instances/{id}/chat (F-2.3).
// Every other method is unused by these tests.
type agentFakeService struct {
	chatEntries []domain.ChatLogEntry
	chatErr     error

	lastChatID     string
	lastChatLimit  int
	lastChatBefore int64
	lastChatQ      string
}

func (f *agentFakeService) Info(context.Context, string, bool) (*domain.AgentInfo, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}

func (f *agentFakeService) Command(context.Context, string, domain.AgentCommandRequest) (*domain.AgentCommandResult, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}

func (f *agentFakeService) Catalog(context.Context, string) (*domain.AgentCatalog, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}

func (f *agentFakeService) Chat(context.Context, string, int64, int) (*domain.AgentChat, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}

func (f *agentFakeService) Map(context.Context, string, bool) (*domain.InstanceMap, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}

func (f *agentFakeService) MapPNG(context.Context, string, bool) (string, *domain.MapInfo, error) {
	return "", nil, domain.E(domain.CodeInternal, "unused")
}

func (f *agentFakeService) TilePNG(context.Context, string, int, int, int, bool) ([]byte, string, *domain.MapInfo, error) {
	return nil, "", nil, domain.E(domain.CodeInternal, "unused")
}

func (f *agentFakeService) WaterMaskPNG(context.Context, string) ([]byte, string, error) {
	return nil, "", domain.E(domain.CodeInternal, "unused")
}

func (f *agentFakeService) ExploredPNG(context.Context, string) (string, *domain.ExploredInfo, error) {
	return "", nil, domain.E(domain.CodeInternal, "unused")
}

func (f *agentFakeService) RenderMap(context.Context, string, domain.MapRenderRequest) (*domain.MapInfo, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}

func (f *agentFakeService) ChatHistory(_ context.Context, instanceID string, limit int, before int64, q string) ([]domain.ChatLogEntry, error) {
	f.lastChatID, f.lastChatLimit, f.lastChatBefore, f.lastChatQ = instanceID, limit, before, q
	if f.chatErr != nil {
		return nil, f.chatErr
	}
	return f.chatEntries, nil
}

// newAgentTestDeps wires a Deps with the agentFakeService and a fixed-role
// user (see systemFakeAuth in system_handlers_test.go, same package).
func newAgentTestDeps(t *testing.T, role domain.Role) (*Deps, *agentFakeService) {
	t.Helper()
	svc := &agentFakeService{}
	d := &Deps{
		Log:   slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 4})),
		Agent: svc,
		Auth:  systemFakeAuth{user: &domain.User{ID: 1, Username: "tester", Role: role}},
	}
	return d, svc
}

func TestGetChatHistoryHandler_ReturnsEntriesAndDefaultsLimit(t *testing.T) {
	d, svc := newAgentTestDeps(t, domain.RoleViewer)
	svc.chatEntries = []domain.ChatLogEntry{
		{ID: 2, InstanceID: "main", Sender: "Bjorn", Text: "gg", Type: "normal"},
		{ID: 1, InstanceID: "main", Sender: "Freya", Text: "hi", Type: "normal"},
	}
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/chat", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Entries []domain.ChatLogEntry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Entries) != 2 || body.Entries[0].Text != "gg" {
		t.Fatalf("unexpected entries: %+v", body.Entries)
	}
	if svc.lastChatID != "main" {
		t.Fatalf("expected the service to be called with instance id main, got %q", svc.lastChatID)
	}
	if svc.lastChatLimit != defaultChatLimit {
		t.Fatalf("expected the default limit %d, got %d", defaultChatLimit, svc.lastChatLimit)
	}
	if svc.lastChatBefore != 0 {
		t.Fatalf("expected no before cursor, got %d", svc.lastChatBefore)
	}
}

func TestGetChatHistoryHandler_PassesLimitBeforeAndQ(t *testing.T) {
	d, svc := newAgentTestDeps(t, domain.RoleViewer)
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/chat?limit=5&before=42&q=gg", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.lastChatLimit != 5 {
		t.Fatalf("expected limit 5, got %d", svc.lastChatLimit)
	}
	if svc.lastChatBefore != 42 {
		t.Fatalf("expected before 42, got %d", svc.lastChatBefore)
	}
	if svc.lastChatQ != "gg" {
		t.Fatalf("expected q %q, got %q", "gg", svc.lastChatQ)
	}
}

func TestGetChatHistoryHandler_LimitAboveMaxIsCapped(t *testing.T) {
	d, svc := newAgentTestDeps(t, domain.RoleViewer)
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/chat?limit=10000", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.lastChatLimit != maxChatLimit {
		t.Fatalf("expected the limit capped at %d, got %d", maxChatLimit, svc.lastChatLimit)
	}
}

func TestGetChatHistoryHandler_InvalidLimit(t *testing.T) {
	d, _ := newAgentTestDeps(t, domain.RoleViewer)
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/chat?limit=abc", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for a non-numeric limit, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetChatHistoryHandler_InvalidBefore(t *testing.T) {
	d, _ := newAgentTestDeps(t, domain.RoleViewer)
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/chat?before=abc", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for a non-numeric before, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetChatHistoryHandler_ServiceErrorPropagates(t *testing.T) {
	d, svc := newAgentTestDeps(t, domain.RoleViewer)
	svc.chatErr = domain.E(domain.CodeInternal, "boom")
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/chat", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetChatHistoryHandler_ServiceNotConfigured(t *testing.T) {
	d, _ := newAgentTestDeps(t, domain.RoleViewer)
	d.Agent = nil
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/main/chat", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when the agent service is not configured, got %d: %s", rec.Code, rec.Body.String())
	}
}
