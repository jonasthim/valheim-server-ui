package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
	"github.com/jonasthim/valheim-server-ui/internal/scheduler"
)

// schedulesMemDBCounter gives each schedules handler test its own uniquely
// named SQLite shared-cache in-memory database, so a background goroutine
// from one test (e.g. the scheduler's async job-result bookkeeping) that
// hasn't finished with its connection yet can never bleed rows into the
// next test's database — see the identical note in
// internal/scheduler/testutil_test.go.
var schedulesMemDBCounter int64

func newSchedulesMemoryDB(t *testing.T) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("schedules_api_test_%d", atomic.AddInt64(&schedulesMemDBCounter, 1))
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", name)
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	sqldb.SetMaxOpenConns(1)
	if err := db.Migrate(context.Background(), sqldb); err != nil {
		t.Fatalf("migrate memory db: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	return sqldb
}

// fakeSchedulerPlayers is a domain.PlayerCounter test double for the
// scheduler's only_when_empty checks.
type fakeSchedulerPlayers struct {
	mu     sync.Mutex
	counts map[string]int
	known  map[string]bool
}

func newFakeSchedulerPlayers() *fakeSchedulerPlayers {
	return &fakeSchedulerPlayers{counts: map[string]int{}, known: map[string]bool{}}
}

func (f *fakeSchedulerPlayers) set(instanceID string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[instanceID] = n
	f.known[instanceID] = true
}

func (f *fakeSchedulerPlayers) PlayersOnline(_ context.Context, instanceID string) (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.known[instanceID] {
		return 0, false
	}
	return f.counts[instanceID], true
}

// schedulesTestAPI wires a real instance.Service (WP-02) and a real
// scheduler.Service (WP-07, the package under test) behind NewRouter, using
// the fakeAuthenticator/fakeAPISupervisor already defined in
// instances_handlers_test.go so role-gating and instance CRUD go through
// real code.
type schedulesTestAPI struct {
	handler http.Handler
	inst    *instance.Service
	runner  *jobs.Runner
	players *fakeSchedulerPlayers

	backupCalls int
	updateCalls int
	available   bool
	failNextJob bool
}

func newSchedulesTestAPI(t *testing.T) *schedulesTestAPI {
	t.Helper()
	sqldb := newSchedulesMemoryDB(t)

	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Supervisor = "direct"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	sup := newFakeAPISupervisor()
	inst := instance.New(sqldb, nil, sup, cfg, log)
	runner := jobs.New(sqldb, nil, t.TempDir(), log)
	players := newFakeSchedulerPlayers()

	a := &schedulesTestAPI{inst: inst, runner: runner, players: players}

	hooks := scheduler.Hooks{
		Backup: func(ctx context.Context, instanceID, requestedBy string) (*domain.Job, error) {
			a.backupCalls++
			return runner.Enqueue(ctx, jobs.Spec{
				Type: domain.JobBackup, InstanceID: instanceID, Title: "Test backup", RequestedBy: requestedBy,
			}, func(context.Context, *jobs.Logger) error {
				if a.failNextJob {
					return context.DeadlineExceeded
				}
				return nil
			})
		},
		Update: func(ctx context.Context, instanceID, requestedBy string, _ bool) (*domain.Job, error) {
			a.updateCalls++
			return runner.Enqueue(ctx, jobs.Spec{
				Type: domain.JobUpdate, InstanceID: instanceID, Title: "Test update", RequestedBy: requestedBy,
			}, func(context.Context, *jobs.Logger) error { return nil })
		},
		UpdateAvailable: func(context.Context, string) (bool, error) { return a.available, nil },
	}
	svc := scheduler.New(sqldb, inst, runner, players, hooks, log)

	deps := &Deps{
		Cfg:       cfg,
		Log:       log,
		DB:        sqldb,
		Auth:      fakeAuthenticator{},
		Instances: inst,
		Schedules: svc,
	}
	a.handler = NewRouter(deps, nil)
	return a
}

func (a *schedulesTestAPI) createInstance(t *testing.T, id string, port int) {
	t.Helper()
	cfg := domain.InstanceConfig{Name: "Test " + id, World: "Dedicated", Password: "secret123", Port: port, Public: true}
	cfg.ApplyDefaults()
	if _, err := a.inst.Create(context.Background(), id, "Test "+id, cfg, false); err != nil {
		t.Fatalf("create instance %s: %v", id, err)
	}
}

func (a *schedulesTestAPI) do(t *testing.T, method, path, role string, body any) *httptest.ResponseRecorder {
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

func validScheduleReq(kind, cron string, enabled bool) map[string]any {
	return map[string]any{
		"kind":            kind,
		"cron":            cron,
		"enabled":         enabled,
		"only_when_empty": true,
		"note":            "test",
	}
}

func TestSchedules_CreateListUpdateDelete(t *testing.T) {
	a := newSchedulesTestAPI(t)
	a.createInstance(t, "main", 2456)

	// Empty list to start.
	rec := a.do(t, http.MethodGet, "/api/v1/instances/main/schedules", "viewer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Schedules []domain.Schedule `json:"schedules"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listResp.Schedules) != 0 {
		t.Fatalf("expected empty list, got %+v", listResp.Schedules)
	}

	// Viewer cannot create.
	rec = a.do(t, http.MethodPost, "/api/v1/instances/main/schedules", "viewer", validScheduleReq("backup", "0 3 * * *", true))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer create: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}

	// Operator creates.
	rec = a.do(t, http.MethodPost, "/api/v1/instances/main/schedules", "operator", validScheduleReq("backup", "0 3 * * *", true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp struct {
		Schedule domain.Schedule `json:"schedule"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	sc := createResp.Schedule
	if sc.ID == 0 || sc.InstanceID != "main" || sc.Kind != domain.ScheduleBackup || sc.Cron != "0 3 * * *" {
		t.Fatalf("unexpected created schedule: %+v", sc)
	}
	if sc.NextRunAt == nil {
		t.Errorf("expected next_run_at to be populated for an enabled schedule")
	}

	// Invalid create body -> 422.
	rec = a.do(t, http.MethodPost, "/api/v1/instances/main/schedules", "operator", validScheduleReq("bogus", "0 3 * * *", true))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid create: expected 422, got %d: %s", rec.Code, rec.Body.String())
	}

	// Update.
	updatePath := "/api/v1/instances/main/schedules/" + strconv.FormatInt(sc.ID, 10)
	rec = a.do(t, http.MethodPatch, updatePath, "operator", validScheduleReq("restart", "@hourly", false))
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var updateResp struct {
		Schedule domain.Schedule `json:"schedule"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updateResp.Schedule.Kind != domain.ScheduleRestart || updateResp.Schedule.Cron != "@hourly" || updateResp.Schedule.Enabled {
		t.Fatalf("unexpected updated schedule: %+v", updateResp.Schedule)
	}

	// Delete.
	rec = a.do(t, http.MethodDelete, updatePath, "operator", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = a.do(t, http.MethodGet, "/api/v1/instances/main/schedules", "viewer", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if len(listResp.Schedules) != 0 {
		t.Fatalf("expected empty list after delete, got %+v", listResp.Schedules)
	}
}

func TestSchedules_UnknownInstance404(t *testing.T) {
	a := newSchedulesTestAPI(t)
	rec := a.do(t, http.MethodGet, "/api/v1/instances/does-not-exist/schedules", "viewer", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSchedules_BadScheduleID404(t *testing.T) {
	a := newSchedulesTestAPI(t)
	a.createInstance(t, "main", 2456)
	rec := a.do(t, http.MethodPatch, "/api/v1/instances/main/schedules/not-a-number", "operator", validScheduleReq("backup", "@daily", true))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a non-numeric schedule id, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = a.do(t, http.MethodDelete, "/api/v1/instances/main/schedules/999999", "operator", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown schedule id, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSchedules_RunNow_Succeeds(t *testing.T) {
	a := newSchedulesTestAPI(t)
	a.createInstance(t, "main", 2456)

	rec := a.do(t, http.MethodPost, "/api/v1/instances/main/schedules", "operator", validScheduleReq("backup", "@daily", true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp struct {
		Schedule domain.Schedule `json:"schedule"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &createResp)

	// Viewer cannot run.
	runPath := "/api/v1/instances/main/schedules/" + strconv.FormatInt(createResp.Schedule.ID, 10) + "/run"
	rec = a.do(t, http.MethodPost, runPath, "viewer", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer run: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = a.do(t, http.MethodPost, runPath, "operator", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("run: expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var runResp struct {
		Job domain.Job `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &runResp); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if runResp.Job.ID == "" || runResp.Job.Type != domain.JobBackup {
		t.Fatalf("unexpected job: %+v", runResp.Job)
	}
	if a.backupCalls != 1 {
		t.Fatalf("expected the backup hook to be called once, got %d", a.backupCalls)
	}
}

func TestSchedules_RunNow_SkippedIsConflict(t *testing.T) {
	a := newSchedulesTestAPI(t)
	a.createInstance(t, "main", 2456)
	a.players.set("main", 5)

	rec := a.do(t, http.MethodPost, "/api/v1/instances/main/schedules", "operator", validScheduleReq("restart", "@daily", true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp struct {
		Schedule domain.Schedule `json:"schedule"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &createResp)

	runPath := "/api/v1/instances/main/schedules/" + strconv.FormatInt(createResp.Schedule.ID, 10) + "/run"
	rec = a.do(t, http.MethodPost, runPath, "operator", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("run (skipped): expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSchedules_CrossInstanceScheduleIs404(t *testing.T) {
	a := newSchedulesTestAPI(t)
	a.createInstance(t, "main", 2456)
	a.createInstance(t, "other", 2460)

	rec := a.do(t, http.MethodPost, "/api/v1/instances/main/schedules", "operator", validScheduleReq("backup", "@daily", true))
	var createResp struct {
		Schedule domain.Schedule `json:"schedule"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &createResp)

	otherPath := "/api/v1/instances/other/schedules/" + strconv.FormatInt(createResp.Schedule.ID, 10)
	rec = a.do(t, http.MethodDelete, otherPath, "operator", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 deleting a schedule that belongs to a different instance, got %d: %s", rec.Code, rec.Body.String())
	}
}
