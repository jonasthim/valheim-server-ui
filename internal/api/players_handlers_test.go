package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/players"
)

// fakeAuditor records every audit call for assertions, without depending on
// the real WP-01 audit package.
type fakeAuditor struct {
	mu    sync.Mutex
	calls []auditCall
}

type auditCall struct {
	action     string
	instanceID string
	target     string
	details    map[string]any
}

func (a *fakeAuditor) Record(_ *http.Request, action, instanceID, target string, details map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, auditCall{action: action, instanceID: instanceID, target: target, details: details})
}

// List satisfies the Auditor interface; these tests only assert on Record calls.
func (a *fakeAuditor) List(context.Context, string, string, int, int64) ([]domain.AuditEntry, error) {
	return nil, nil
}

func (a *fakeAuditor) last() (auditCall, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.calls) == 0 {
		return auditCall{}, false
	}
	return a.calls[len(a.calls)-1], true
}

// newPlayersTestRouter builds a router with a real players.Service backed by
// a fake instance layout in a temp dir (WP-02's instance service is not
// depended on), and deps.Auth left nil so devFakeAdmin injects an admin user
// for every request.
func newPlayersTestRouter(t *testing.T) (http.Handler, string, *fakeAuditor) {
	t.Helper()
	root := t.TempDir()
	knownID := "main"

	paths := func(id string) domain.InstancePaths {
		save := filepath.Join(root, id, "save")
		return domain.InstancePaths{
			Root: filepath.Join(root, id),
			Save: save,
		}
	}
	exists := func(_ context.Context, id string) (bool, error) {
		return id == knownID, nil
	}

	svc := players.NewService(nil, players.NewSQLStore(nil), paths, exists)
	auditor := &fakeAuditor{}

	deps := &Deps{
		Log:     slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Players: svc,
		Audit:   auditor,
	}
	handler := NewRouter(deps, nil)
	return handler, root, auditor
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if method != http.MethodGet {
		req.Header.Set(CSRFHeader, CSRFHeaderValue)
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPlayersHandler_GetPlayers_UnknownInstance404(t *testing.T) {
	h, _, _ := newPlayersTestRouter(t)
	rec := doJSON(t, h, http.MethodGet, "/api/v1/instances/does-not-exist/players", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPlayersHandler_GetPlayers_Empty(t *testing.T) {
	h, _, _ := newPlayersTestRouter(t)
	rec := doJSON(t, h, http.MethodGet, "/api/v1/instances/main/players", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp domain.PlayersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.CountSource != "none" {
		t.Errorf("CountSource = %q, want none (no tracker attached)", resp.CountSource)
	}
	if len(resp.Online) != 0 || len(resp.Known) != 0 {
		t.Errorf("resp = %#v, want empty online/known", resp)
	}
}

func TestPlayersHandler_ListKind_InvalidIs404(t *testing.T) {
	h, _, _ := newPlayersTestRouter(t)
	rec := doJSON(t, h, http.MethodGet, "/api/v1/instances/main/lists/not-a-kind", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPlayersHandler_GetList_EmptyWhenFileMissing(t *testing.T) {
	h, _, _ := newPlayersTestRouter(t)
	rec := doJSON(t, h, http.MethodGet, "/api/v1/instances/main/lists/admin", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var list domain.PlayerList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if list.Kind != domain.ListAdmin || len(list.Entries) != 0 {
		t.Fatalf("list = %#v, want empty admin list", list)
	}
}

func TestPlayersHandler_PutList_RoundTripsAndAudits(t *testing.T) {
	h, root, auditor := newPlayersTestRouter(t)

	body := domain.PlayerList{
		Kind: domain.ListBanned,
		Entries: []domain.PlayerListEntry{
			{ID: "76561198000000001", Comment: "griefer"},
			{ID: "76561198000000002"},
		},
	}
	rec := doJSON(t, h, http.MethodPut, "/api/v1/instances/main/lists/banned", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got domain.PlayerList
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("Entries = %#v, want 2", got.Entries)
	}

	// The file must actually exist under the instance's save dir.
	if _, err := os.Stat(filepath.Join(root, "main", "save", "bannedlist.txt")); err != nil {
		t.Fatalf("bannedlist.txt not written: %v", err)
	}

	// A subsequent GET reflects the write.
	rec = doJSON(t, h, http.MethodGet, "/api/v1/instances/main/lists/banned", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var reread domain.PlayerList
	if err := json.Unmarshal(rec.Body.Bytes(), &reread); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(reread.Entries) != 2 || reread.Entries[0].Comment != "griefer" {
		t.Fatalf("reread = %#v, want 2 entries with preserved comment", reread.Entries)
	}

	call, ok := auditor.last()
	if !ok {
		t.Fatalf("no audit call recorded for PUT")
	}
	if call.action != "players.list.update" {
		t.Errorf("audit action = %q, want players.list.update", call.action)
	}
	if call.instanceID != "main" {
		t.Errorf("audit instanceID = %q, want main", call.instanceID)
	}
	if call.details["kind"] != "banned" {
		t.Errorf("audit details[kind] = %v, want banned", call.details["kind"])
	}
	if call.details["count"] != 2 {
		t.Errorf("audit details[count] = %v, want 2", call.details["count"])
	}
}

func TestPlayersHandler_PutList_ValidationFailure(t *testing.T) {
	h, _, _ := newPlayersTestRouter(t)
	body := domain.PlayerList{
		Kind:    domain.ListPermitted,
		Entries: []domain.PlayerListEntry{{ID: "not valid!!"}},
	}
	rec := doJSON(t, h, http.MethodPut, "/api/v1/instances/main/lists/permitted", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPlayersHandler_PutList_UnknownInstance404(t *testing.T) {
	h, _, _ := newPlayersTestRouter(t)
	body := domain.PlayerList{Kind: domain.ListAdmin}
	rec := doJSON(t, h, http.MethodPut, "/api/v1/instances/does-not-exist/lists/admin", body)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}
