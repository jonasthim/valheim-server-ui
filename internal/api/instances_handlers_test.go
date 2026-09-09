package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// fakeAuthenticator lets tests pick the acting user's role per-request via the
// X-Test-Role header, instead of exercising the real WP-01 auth stack.
type fakeAuthenticator struct{}

func (fakeAuthenticator) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := domain.Role(r.Header.Get("X-Test-Role"))
		if !role.Valid() {
			next.ServeHTTP(w, r)
			return
		}
		u := &domain.User{ID: 1, Username: "tester", Role: role}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}

// fakeAPISupervisor is a minimal in-memory supervisor.Supervisor for handler
// tests, so /start /stop /status don't need a real spawned process.
type fakeAPISupervisor struct {
	mu     sync.Mutex
	status map[string]supervisor.Status
}

func newFakeAPISupervisor() *fakeAPISupervisor {
	return &fakeAPISupervisor{status: map[string]supervisor.Status{}}
}
func (f *fakeAPISupervisor) Kind() string { return "fake" }
func (f *fakeAPISupervisor) Start(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = supervisor.Status{State: supervisor.StateRunning, PID: 123, Since: time.Now()}
	return nil
}
func (f *fakeAPISupervisor) Stop(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = supervisor.Status{State: supervisor.StateStopped}
	return nil
}
func (f *fakeAPISupervisor) Restart(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = supervisor.Status{State: supervisor.StateRunning, PID: 456, Since: time.Now()}
	return nil
}
func (f *fakeAPISupervisor) Status(ctx context.Context, id string) (supervisor.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.status[id]
	if st.State == "" {
		st.State = supervisor.StateStopped
	}
	return st, nil
}
func (f *fakeAPISupervisor) SetAutostart(ctx context.Context, id string, on bool) error { return nil }

// setRunning forces id into the running state, used to test the delete/409
// guard without going through Start (which requires an installed binary).
func (f *fakeAPISupervisor) setRunning(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = supervisor.Status{State: supervisor.StateRunning, PID: 1}
}

type testAPI struct {
	handler http.Handler
	sup     *fakeAPISupervisor
	svc     *instance.Service
}

func newTestAPI(t *testing.T) *testAPI {
	t.Helper()
	ctx := context.Background()
	sqldb, err := db.OpenMemory(ctx)
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Supervisor = "direct"

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := events.NewBus()
	sup := newFakeAPISupervisor()
	svc := instance.New(sqldb, bus, sup, cfg, log)

	deps := &Deps{
		Cfg:        cfg,
		Log:        log,
		DB:         sqldb,
		Bus:        bus,
		Supervisor: sup,
		Auth:       fakeAuthenticator{},
		Instances:  svc,
	}
	return &testAPI{handler: NewRouter(deps, nil), sup: sup, svc: svc}
}

func (a *testAPI) do(t *testing.T, method, path, role string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if role != "" {
		req.Header.Set("X-Test-Role", role)
	}
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set(CSRFHeader, CSRFHeaderValue)
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

func validCreateReq(id string, port int) map[string]any {
	return map[string]any{
		"id":   id,
		"name": "Test Instance",
		"config": map[string]any{
			"name":      "In-Game Name",
			"world":     "Dedicated",
			"password":  "s3cret12",
			"port":      port,
			"public":    true,
			"crossplay": false,
		},
	}
}

func TestInstances_CreateGetListDelete(t *testing.T) {
	api := newTestAPI(t)

	rec := api.do(t, http.MethodPost, "/api/v1/instances", "admin", validCreateReq("main", 2456))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Instance domain.Instance `json:"instance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Instance.ID != "main" {
		t.Errorf("expected id=main, got %q", created.Instance.ID)
	}
	if created.Instance.Config.Password != "s3cret12" {
		t.Errorf("admin should see the real password, got %q", created.Instance.Config.Password)
	}

	// Viewer sees the masked password.
	rec = api.do(t, http.MethodGet, "/api/v1/instances/main", "viewer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Instance domain.Instance `json:"instance"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Instance.Config.Password != "********" {
		t.Errorf("viewer should see a masked password, got %q", got.Instance.Config.Password)
	}

	// List.
	rec = api.do(t, http.MethodGet, "/api/v1/instances", "viewer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}
	var list struct {
		Instances []domain.Instance `json:"instances"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(list.Instances))
	}

	// Delete requires admin.
	rec = api.do(t, http.MethodDelete, "/api/v1/instances/main", "operator", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("operator delete: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = api.do(t, http.MethodDelete, "/api/v1/instances/main", "admin", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin delete: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = api.do(t, http.MethodGet, "/api/v1/instances/main", "viewer", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: expected 404, got %d", rec.Code)
	}
}

func TestInstances_CreateRequiresAdmin(t *testing.T) {
	api := newTestAPI(t)
	rec := api.do(t, http.MethodPost, "/api/v1/instances", "operator", validCreateReq("main", 2456))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for operator create, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInstances_CreateValidationError(t *testing.T) {
	api := newTestAPI(t)
	req := validCreateReq("main", 2456)
	req["config"].(map[string]any)["password"] = "no"
	rec := api.do(t, http.MethodPost, "/api/v1/instances", "admin", req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "validation_failed" {
		t.Errorf("expected validation_failed code, got %v", body)
	}
}

func TestInstances_PortOverlapConflict(t *testing.T) {
	api := newTestAPI(t)
	if rec := api.do(t, http.MethodPost, "/api/v1/instances", "admin", validCreateReq("a", 2456)); rec.Code != http.StatusCreated {
		t.Fatalf("create a: %d %s", rec.Code, rec.Body.String())
	}
	rec := api.do(t, http.MethodPost, "/api/v1/instances", "admin", validCreateReq("b", 2457))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 port_in_use, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInstances_PatchRequiresOperator(t *testing.T) {
	api := newTestAPI(t)
	api.do(t, http.MethodPost, "/api/v1/instances", "admin", validCreateReq("main", 2456))

	rec := api.do(t, http.MethodPatch, "/api/v1/instances/main", "viewer", map[string]any{"name": "New Name"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer patch: expected 403, got %d", rec.Code)
	}

	rec = api.do(t, http.MethodPatch, "/api/v1/instances/main", "operator", map[string]any{"name": "New Name"})
	if rec.Code != http.StatusOK {
		t.Fatalf("operator patch: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Instance domain.Instance `json:"instance"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Instance.Name != "New Name" {
		t.Errorf("expected renamed instance, got %+v", got.Instance)
	}
}

func TestInstances_DeleteRefusesWhileRunning(t *testing.T) {
	api := newTestAPI(t)
	api.do(t, http.MethodPost, "/api/v1/instances", "admin", validCreateReq("main", 2456))
	api.sup.setRunning("main")

	rec := api.do(t, http.MethodDelete, "/api/v1/instances/main", "admin", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 instance_running, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInstances_StartNotInstalled(t *testing.T) {
	api := newTestAPI(t)
	api.do(t, http.MethodPost, "/api/v1/instances", "admin", validCreateReq("main", 2456))

	rec := api.do(t, http.MethodPost, "/api/v1/instances/main/start", "operator", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 instance_not_installed, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInstances_StartStopStatus(t *testing.T) {
	api := newTestAPI(t)
	api.do(t, http.MethodPost, "/api/v1/instances", "admin", validCreateReq("main", 2456))

	// Mark installed by touching the server binary the service checks for.
	paths := api.svc.Paths("main")
	if err := writeExecutable(paths.ServerBinary()); err != nil {
		t.Fatal(err)
	}

	rec := api.do(t, http.MethodPost, "/api/v1/instances/main/start", "operator", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("start: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var st struct {
		Status domain.InstanceStatus `json:"status"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Status.State != domain.StateRunning {
		t.Errorf("expected running, got %v", st.Status.State)
	}

	rec = api.do(t, http.MethodGet, "/api/v1/instances/main/status", "viewer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", rec.Code)
	}

	rec = api.do(t, http.MethodPost, "/api/v1/instances/main/stop", "operator", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInstances_LogsEmpty(t *testing.T) {
	api := newTestAPI(t)
	api.do(t, http.MethodPost, "/api/v1/instances", "admin", validCreateReq("main", 2456))

	rec := api.do(t, http.MethodGet, "/api/v1/instances/main/logs", "viewer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("logs: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Lines []string `json:"lines"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Lines) != 0 {
		t.Errorf("expected no lines for a fresh instance, got %v", body.Lines)
	}
}

func TestInstances_Unauthenticated(t *testing.T) {
	api := newTestAPI(t)
	rec := api.do(t, http.MethodGet, "/api/v1/instances", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func writeExecutable(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755)
}
