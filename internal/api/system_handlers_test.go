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
