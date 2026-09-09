package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// systemFakeSteamService implements SteamService for handler tests; only the
// methods system_handlers.go calls need real behaviour.
type systemFakeSteamService struct {
	installed bool
	buildID   string
	checkedAt *time.Time
}

func (f *systemFakeSteamService) EnqueueInstall(ctx context.Context, instanceID, requestedBy string) (*domain.Job, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f *systemFakeSteamService) EnqueueUpdate(ctx context.Context, instanceID, requestedBy string, stopIfRunning bool) (*domain.Job, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f *systemFakeSteamService) CheckUpdate(ctx context.Context, instanceID string) (*domain.UpdateInfo, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f *systemFakeSteamService) SteamCMDInstalled() bool { return f.installed }
func (f *systemFakeSteamService) LatestBuildID() (string, *domain.UpdateInfo) {
	if f.buildID == "" {
		return "", nil
	}
	return f.buildID, &domain.UpdateInfo{LatestBuildID: f.buildID, CheckedAt: f.checkedAt}
}

func newSystemTestDeps(t *testing.T, dataDir string) *Deps {
	t.Helper()
	return &Deps{
		Cfg:       config.Config{DataDir: dataDir, Supervisor: "direct"},
		Log:       slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 4})),
		Version:   "v1.2.3",
		Commit:    "abc1234",
		StartedAt: time.Now().UTC(),
	}
}

// systemFakeAuth injects a fixed user, bypassing real authentication (see
// jobsFakeAuth in jobs_handlers_test.go for the pattern this mirrors).
type systemFakeAuth struct{ user *domain.User }

func (f systemFakeAuth) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), f.user)))
	})
}
func (f systemFakeAuth) Setup(ctx context.Context, w http.ResponseWriter, r *http.Request, username, password, displayName, email string) (*domain.User, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f systemFakeAuth) Login(ctx context.Context, w http.ResponseWriter, r *http.Request, username, password string) (*domain.User, error) {
	return nil, domain.E(domain.CodeInternal, "unused")
}
func (f systemFakeAuth) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request) {}
func (f systemFakeAuth) OIDCLogin(w http.ResponseWriter, r *http.Request)                   {}
func (f systemFakeAuth) OIDCCallback(w http.ResponseWriter, r *http.Request)                {}

// systemFakeAuditor records calls for assertions.
type systemFakeAuditor struct {
	calls []systemAuditCall
}

type systemAuditCall struct {
	action, instanceID, target string
	details                    map[string]any
}

func (f *systemFakeAuditor) Record(r *http.Request, action, instanceID, target string, details map[string]any) {
	f.calls = append(f.calls, systemAuditCall{action, instanceID, target, details})
}
func (f *systemFakeAuditor) List(ctx context.Context, instanceID, username string, limit int, before int64) ([]domain.AuditEntry, error) {
	return nil, nil
}

// systemFakeSelfUpdateService implements SelfUpdateService for handler tests.
type systemFakeSelfUpdateService struct {
	info       *domain.AppUpdateInfo
	checkErr   error
	enqueueJob *domain.Job
	enqueueErr error

	lastEnqueueVersion string
}

func (f *systemFakeSelfUpdateService) Info(ctx context.Context) *domain.AppUpdateInfo {
	return f.info
}

func (f *systemFakeSelfUpdateService) CheckNow(ctx context.Context) (*domain.AppUpdateInfo, error) {
	if f.checkErr != nil {
		return nil, f.checkErr
	}
	return f.info, nil
}

func (f *systemFakeSelfUpdateService) EnqueueUpgrade(ctx context.Context, version, requestedBy string) (*domain.Job, error) {
	f.lastEnqueueVersion = version
	if f.enqueueErr != nil {
		return nil, f.enqueueErr
	}
	return f.enqueueJob, nil
}

func newSystemAdminDeps(t *testing.T) (*Deps, *systemFakeSelfUpdateService, *systemFakeAuditor) {
	t.Helper()
	su := &systemFakeSelfUpdateService{}
	audit := &systemFakeAuditor{}
	d := newSystemTestDeps(t, t.TempDir())
	d.SelfUpdate = su
	d.Audit = audit
	d.Auth = systemFakeAuth{user: &domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}
	return d, su, audit
}

func TestSystemHandler_IncludesAppUpdate(t *testing.T) {
	d := newSystemTestDeps(t, t.TempDir())
	checkedAt := time.Now().UTC()
	d.SelfUpdate = &systemFakeSelfUpdateService{info: &domain.AppUpdateInfo{
		CurrentVersion: "v1.2.3", LatestVersion: "v1.3.0", UpdateAvailable: true,
		CanSelfUpgrade: true, CheckedAt: &checkedAt,
	}}
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var info systemInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.AppUpdate == nil || info.AppUpdate.LatestVersion != "v1.3.0" || !info.AppUpdate.UpdateAvailable {
		t.Fatalf("expected app_update to be populated, got %+v", info.AppUpdate)
	}
}

func TestSystemHandler_OmitsAppUpdateWhenNotConfigured(t *testing.T) {
	d := newSystemTestDeps(t, t.TempDir())
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if jsonHasKey(rec.Body.String(), "app_update") {
		t.Fatalf("expected app_update to be omitted (omitempty, nil pointer), got %s", rec.Body.String())
	}
}

// jsonHasKey reports whether the top-level JSON object body has key.
func jsonHasKey(body, key string) bool {
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func TestSystemUpdateCheckHandler(t *testing.T) {
	d, su, audit := newSystemAdminDeps(t)
	su.info = &domain.AppUpdateInfo{CurrentVersion: "v1.2.3", LatestVersion: "v1.3.0", UpdateAvailable: true}
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/update-check", nil)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var info domain.AppUpdateInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.LatestVersion != "v1.3.0" {
		t.Fatalf("expected latest_version v1.3.0, got %+v", info)
	}
	if len(audit.calls) != 1 || audit.calls[0].action != "app.update_check" {
		t.Fatalf("expected one app.update_check audit record, got %+v", audit.calls)
	}
}

func TestSystemUpdateCheckHandler_UpstreamError(t *testing.T) {
	d, su, _ := newSystemAdminDeps(t)
	su.checkErr = domain.E(domain.CodeUpstreamError, "github unreachable")
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/update-check", nil)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSystemUpdateCheckHandler_RequiresAdmin(t *testing.T) {
	d, _, _ := newSystemAdminDeps(t)
	d.Auth = systemFakeAuth{user: &domain.User{ID: 2, Username: "viewer", Role: domain.RoleViewer}}
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/update-check", nil)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a viewer, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSystemUpgradeHandler_Latest(t *testing.T) {
	d, su, audit := newSystemAdminDeps(t)
	su.enqueueJob = &domain.Job{ID: "job-1", Type: domain.JobSelfUpgrade, Status: domain.JobQueued}
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/upgrade", nil)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	job := body["job"].(map[string]any)
	if job["id"] != "job-1" {
		t.Fatalf("expected job job-1, got %v", job)
	}
	if su.lastEnqueueVersion != "" {
		t.Fatalf("expected empty version (latest) to be passed through, got %q", su.lastEnqueueVersion)
	}
	if len(audit.calls) != 1 || audit.calls[0].action != "app.upgrade" || audit.calls[0].details["version"] != "latest" {
		t.Fatalf("expected one app.upgrade audit record for latest, got %+v", audit.calls)
	}
}

func TestSystemUpgradeHandler_WithVersion(t *testing.T) {
	d, su, audit := newSystemAdminDeps(t)
	su.enqueueJob = &domain.Job{ID: "job-2", Type: domain.JobSelfUpgrade, Status: domain.JobQueued}
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/upgrade", strings.NewReader(`{"version":"v1.5.0"}`))
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if su.lastEnqueueVersion != "v1.5.0" {
		t.Fatalf("expected version v1.5.0 to be passed through, got %q", su.lastEnqueueVersion)
	}
	if audit.calls[0].details["version"] != "v1.5.0" {
		t.Fatalf("expected audit version v1.5.0, got %+v", audit.calls[0].details)
	}
}

func TestSystemUpgradeHandler_Conflict(t *testing.T) {
	d, su, _ := newSystemAdminDeps(t)
	su.enqueueErr = domain.E(domain.CodeConflict, "a self-upgrade job is already active")
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/upgrade", nil)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSystemUpgradeHandler_RequiresAdmin(t *testing.T) {
	d, su, _ := newSystemAdminDeps(t)
	su.enqueueJob = &domain.Job{ID: "job-3", Type: domain.JobSelfUpgrade}
	d.Auth = systemFakeAuth{user: &domain.User{ID: 2, Username: "operator", Role: domain.RoleOperator}}
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/upgrade", nil)
	req.Header.Set(CSRFHeader, CSRFHeaderValue)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for an operator (upgrade requires admin), got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSystemHandler_NoSteam(t *testing.T) {
	d := newSystemTestDeps(t, t.TempDir())
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var info systemInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.Version != "v1.2.3" || info.Commit != "abc1234" {
		t.Fatalf("unexpected version/commit: %+v", info)
	}
	if info.DataDir != d.Cfg.DataDir {
		t.Fatalf("expected data_dir %s, got %s", d.Cfg.DataDir, info.DataDir)
	}
	if info.Supervisor != "direct" {
		t.Fatalf("expected supervisor direct, got %s", info.Supervisor)
	}
	if info.SteamCMDInstalled {
		t.Fatal("expected steamcmd_installed false when deps.Steam is nil")
	}
	if info.LatestBuildID != "" {
		t.Fatalf("expected no latest_buildid when deps.Steam is nil, got %s", info.LatestBuildID)
	}
	if info.DiskTotalBytes <= 0 || info.DiskFreeBytes < 0 {
		t.Fatalf("expected real disk usage numbers, got total=%d free=%d", info.DiskTotalBytes, info.DiskFreeBytes)
	}
}

func TestSystemHandler_WithSteam(t *testing.T) {
	d := newSystemTestDeps(t, t.TempDir())
	checkedAt := time.Now().UTC().Add(-5 * time.Minute)
	d.Steam = &systemFakeSteamService{installed: true, buildID: "22222222", checkedAt: &checkedAt}
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var info systemInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !info.SteamCMDInstalled {
		t.Fatal("expected steamcmd_installed true")
	}
	if info.LatestBuildID != "22222222" {
		t.Fatalf("expected latest_buildid 22222222, got %s", info.LatestBuildID)
	}
	if info.BuildIDCheckedAt == nil || !info.BuildIDCheckedAt.Equal(checkedAt) {
		t.Fatalf("expected buildid_checked_at %v, got %v", checkedAt, info.BuildIDCheckedAt)
	}
}

func TestSystemHandler_MissingDataDirStillResponds(t *testing.T) {
	d := newSystemTestDeps(t, "/nonexistent/path/for/valheim-ui-test")
	router := NewRouter(d, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even when statfs fails, got %d: %s", rec.Code, rec.Body.String())
	}
	var info systemInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.DiskFreeBytes != 0 || info.DiskTotalBytes != 0 {
		t.Fatalf("expected zeroed disk usage for a missing path, got %+v", info)
	}
}
