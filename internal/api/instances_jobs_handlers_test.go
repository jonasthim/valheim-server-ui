package api

import (
	"bytes"
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

// instJobsFakeSteam implements SteamService for these handler tests with
// scriptable results/errors per method.
type instJobsFakeSteam struct {
	installJob *domain.Job
	installErr error

	updateJob     *domain.Job
	updateErr     error
	gotStopIfRun  bool
	sawStopIfRun  bool
	updateCalledN int

	checkInfo *domain.UpdateInfo
	checkErr  error
}

func (f *instJobsFakeSteam) EnqueueInstall(ctx context.Context, instanceID, requestedBy string) (*domain.Job, error) {
	if f.installErr != nil {
		return nil, f.installErr
	}
	j := *f.installJob
	return &j, nil
}

func (f *instJobsFakeSteam) EnqueueUpdate(ctx context.Context, instanceID, requestedBy string, stopIfRunning bool) (*domain.Job, error) {
	f.gotStopIfRun = stopIfRunning
	f.sawStopIfRun = true
	f.updateCalledN++
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	j := *f.updateJob
	return &j, nil
}

func (f *instJobsFakeSteam) CheckUpdate(ctx context.Context, instanceID string) (*domain.UpdateInfo, error) {
	if f.checkErr != nil {
		return nil, f.checkErr
	}
	info := *f.checkInfo
	return &info, nil
}

func (f *instJobsFakeSteam) SteamCMDInstalled() bool                     { return true }
func (f *instJobsFakeSteam) LatestBuildID() (string, *domain.UpdateInfo) { return "", nil }

func newInstJobsTestDeps(t *testing.T, steamSvc *instJobsFakeSteam) (*Deps, *jobsFakeAuditor) {
	t.Helper()
	audit := &jobsFakeAuditor{}
	d := &Deps{
		Log:   slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 4})),
		Steam: steamSvc,
		Audit: audit,
		// Auth left nil: NewRouter injects a fixed dev admin (devFakeAdmin),
		// which satisfies every role gate exercised here.
	}
	return d, audit
}

func instJobsDo(t *testing.T, router http.Handler, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode body %q: %v", rec.Body.String(), err)
		}
	}
	return rec, out
}

func TestInstanceJobs_Install_Success(t *testing.T) {
	steamSvc := &instJobsFakeSteam{installJob: &domain.Job{ID: "j1", Type: domain.JobInstall, InstanceID: "main", Status: domain.JobQueued}}
	d, audit := newInstJobsTestDeps(t, steamSvc)
	router := NewRouter(d, nil)

	rec, body := instJobsDo(t, router, http.MethodPost, "/api/v1/instances/main/install", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	job, ok := body["job"].(map[string]any)
	if !ok || job["id"] != "j1" || job["type"] != "install" {
		t.Fatalf("unexpected job body: %v", body)
	}
	if len(audit.calls) != 1 || audit.calls[0].action != "instance.install" || audit.calls[0].instanceID != "main" {
		t.Fatalf("expected one instance.install audit record for main, got %+v", audit.calls)
	}
}

func TestInstanceJobs_Install_Conflict(t *testing.T) {
	steamSvc := &instJobsFakeSteam{installErr: domain.Ef(domain.CodeInstanceBusy, "instance %q already has an active job", "main")}
	d, _ := newInstJobsTestDeps(t, steamSvc)
	router := NewRouter(d, nil)

	rec, body := instJobsDo(t, router, http.MethodPost, "/api/v1/instances/main/install", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	errBody, ok := body["error"].(map[string]any)
	if !ok || errBody["code"] != "instance_busy" {
		t.Fatalf("expected instance_busy error, got %v", body)
	}
}

func TestInstanceJobs_Update_DefaultsStopIfRunningFalse(t *testing.T) {
	steamSvc := &instJobsFakeSteam{updateJob: &domain.Job{ID: "j2", Type: domain.JobUpdate, InstanceID: "main", Status: domain.JobQueued}}
	d, audit := newInstJobsTestDeps(t, steamSvc)
	router := NewRouter(d, nil)

	rec, body := instJobsDo(t, router, http.MethodPost, "/api/v1/instances/main/update", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if !steamSvc.sawStopIfRun || steamSvc.gotStopIfRun {
		t.Errorf("expected stop_if_running=false when body omitted, got %v (called=%v)", steamSvc.gotStopIfRun, steamSvc.sawStopIfRun)
	}
	job, ok := body["job"].(map[string]any)
	if !ok || job["id"] != "j2" {
		t.Fatalf("unexpected job body: %v", body)
	}
	if len(audit.calls) != 1 || audit.calls[0].action != "instance.update" {
		t.Fatalf("expected one instance.update audit record, got %+v", audit.calls)
	}
}

func TestInstanceJobs_Update_StopIfRunningTrue(t *testing.T) {
	steamSvc := &instJobsFakeSteam{updateJob: &domain.Job{ID: "j3", Type: domain.JobUpdate, InstanceID: "main", Status: domain.JobQueued}}
	d, _ := newInstJobsTestDeps(t, steamSvc)
	router := NewRouter(d, nil)

	rec, _ := instJobsDo(t, router, http.MethodPost, "/api/v1/instances/main/update", map[string]any{"stop_if_running": true})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if !steamSvc.gotStopIfRun {
		t.Error("expected stop_if_running=true to reach EnqueueUpdate")
	}
}

func TestInstanceJobs_Update_Conflict(t *testing.T) {
	steamSvc := &instJobsFakeSteam{updateErr: domain.Ef(domain.CodeInstanceRunning, "instance %q is running", "main")}
	d, _ := newInstJobsTestDeps(t, steamSvc)
	router := NewRouter(d, nil)

	rec, body := instJobsDo(t, router, http.MethodPost, "/api/v1/instances/main/update", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	errBody, ok := body["error"].(map[string]any)
	if !ok || errBody["code"] != "instance_running" {
		t.Fatalf("expected instance_running error, got %v", body)
	}
}

func TestInstanceJobs_UpdateCheck_Success(t *testing.T) {
	checkedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	steamSvc := &instJobsFakeSteam{checkInfo: &domain.UpdateInfo{
		InstanceID: "main", InstalledBuildID: "111", LatestBuildID: "222", UpdateAvailable: true, CheckedAt: &checkedAt,
	}}
	d, audit := newInstJobsTestDeps(t, steamSvc)
	router := NewRouter(d, nil)

	rec, body := instJobsDo(t, router, http.MethodPost, "/api/v1/instances/main/update-check", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if body["instance_id"] != "main" || body["installed_buildid"] != "111" || body["latest_buildid"] != "222" || body["update_available"] != true {
		t.Fatalf("unexpected UpdateInfo body: %v", body)
	}
	if len(audit.calls) != 1 || audit.calls[0].action != "instance.update_check" {
		t.Fatalf("expected one instance.update_check audit record, got %+v", audit.calls)
	}
}

func TestInstanceJobs_UpdateCheck_NotFound(t *testing.T) {
	steamSvc := &instJobsFakeSteam{checkErr: domain.NotFound("instance")}
	d, _ := newInstJobsTestDeps(t, steamSvc)
	router := NewRouter(d, nil)

	rec, _ := instJobsDo(t, router, http.MethodPost, "/api/v1/instances/main/update-check", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInstanceJobs_InvalidInstanceID(t *testing.T) {
	steamSvc := &instJobsFakeSteam{}
	d, _ := newInstJobsTestDeps(t, steamSvc)
	router := NewRouter(d, nil)

	rec, body := instJobsDo(t, router, http.MethodPost, "/api/v1/instances/Bad_ID!/install", nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
	errBody, ok := body["error"].(map[string]any)
	if !ok || errBody["code"] != "validation_failed" {
		t.Fatalf("expected validation_failed, got %v", body)
	}
}

// TestInstanceJobs_RoleGating uses the X-Test-Role-aware fakeAuthenticator
// (defined in instances_handlers_test.go) instead of the dev fake admin, to
// verify install/update require operator while update-check only requires
// viewer.
func TestInstanceJobs_RoleGating(t *testing.T) {
	steamSvc := &instJobsFakeSteam{
		installJob: &domain.Job{ID: "j1", Type: domain.JobInstall, InstanceID: "main"},
		updateJob:  &domain.Job{ID: "j2", Type: domain.JobUpdate, InstanceID: "main"},
		checkInfo:  &domain.UpdateInfo{InstanceID: "main"},
	}
	d, _ := newInstJobsTestDeps(t, steamSvc)
	d.Auth = fakeAuthenticator{}
	router := NewRouter(d, nil)

	do := func(method, path, role string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set(CSRFHeader, CSRFHeaderValue)
		if role != "" {
			req.Header.Set("X-Test-Role", role)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	if rec := do(http.MethodPost, "/api/v1/instances/main/install", "viewer"); rec.Code != http.StatusForbidden {
		t.Errorf("expected viewer forbidden from install, got %d", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/instances/main/install", "operator"); rec.Code != http.StatusAccepted {
		t.Errorf("expected operator allowed to install, got %d", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/instances/main/update", "viewer"); rec.Code != http.StatusForbidden {
		t.Errorf("expected viewer forbidden from update, got %d", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/instances/main/update-check", "viewer"); rec.Code != http.StatusOK {
		t.Errorf("expected viewer allowed to update-check, got %d", rec.Code)
	}
}
