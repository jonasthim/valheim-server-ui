package instance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
	"github.com/jonasthim/valheim-server-ui/internal/steam"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// fakeSteamCMDPath is the no-network stand-in for steamcmd used by every test
// in this file (testdata/fake-steamcmd.sh).
func fakeSteamCMDPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("testdata", "fake-steamcmd.sh"))
	if err != nil {
		t.Fatalf("resolve fake steamcmd path: %v", err)
	}
	return p
}

// jobsTestEnv wires a Service (fake supervisor/bus), a real jobs.Runner and a
// SteamJobs pointed at the fake steamcmd script.
type jobsTestEnv struct {
	svc    *Service
	sup    *fakeSupervisor
	bus    *fakeBus
	runner *jobs.Runner
	client *steam.Client
	log    *slog.Logger
	sj     *SteamJobs
}

func newJobsTestEnv(t *testing.T, pre PreUpdateBackupFunc) *jobsTestEnv {
	t.Helper()
	svc, sup, bus := newTestService(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := jobs.New(svc.db, bus, filepath.Join(t.TempDir(), "jobs"), log)
	client := steam.New(fakeSteamCMDPath(t), log)
	sj := NewSteamJobs(svc, runner, client, nil, pre, log)
	return &jobsTestEnv{svc: svc, sup: sup, bus: bus, runner: runner, client: client, log: log, sj: sj}
}

// newSteamCMDMissingEnv builds a SteamJobs whose Client points at a
// non-existent steamcmd path, for steamcmd_missing assertions.
func newSteamCMDMissingEnv(t *testing.T) (*Service, *SteamJobs) {
	t.Helper()
	svc, _, bus := newTestService(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := jobs.New(svc.db, bus, filepath.Join(t.TempDir(), "jobs"), log)
	client := steam.New(filepath.Join(t.TempDir(), "no-such-steamcmd.sh"), log)
	return svc, NewSteamJobs(svc, runner, client, nil, nil, log)
}

// ---------------------------------------------------------------- install

func TestSteamJobs_EnqueueInstall_Success(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	ctx := context.Background()
	if _, err := env.svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Setenv("FAKE_STEAMCMD_BUILDID", "10000001")

	job, err := env.sj.EnqueueInstall(ctx, "main", "alice")
	if err != nil {
		t.Fatalf("EnqueueInstall: %v", err)
	}
	if job.Type != domain.JobInstall || job.InstanceID != "main" || job.RequestedBy != "alice" {
		t.Errorf("unexpected job: %+v", job)
	}

	final, err := env.runner.WaitFor(ctx, job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected succeeded, got %v (err=%q)", final.Status, final.Error)
	}
	if final.Summary["buildid"] != "10000001" {
		t.Errorf("expected summary buildid 10000001, got %+v", final.Summary)
	}

	inst, err := env.svc.Get(ctx, "main")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if inst.InstalledBuildID != "10000001" {
		t.Errorf("expected installed_buildid persisted, got %q", inst.InstalledBuildID)
	}

	if _, err := os.Stat(env.svc.Paths("main").AppManifest()); err != nil {
		t.Errorf("expected appmanifest to be written: %v", err)
	}

	found := false
	for _, ev := range env.bus.all() {
		if ev.Name == domain.EventInstanceStatus && ev.InstanceID == "main" {
			found = true
		}
	}
	if !found {
		t.Error("expected an instance.status event to be published after install")
	}
}

func TestSteamJobs_EnqueueInstall_Failure(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	ctx := context.Background()
	if _, err := env.svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Setenv("FAKE_STEAMCMD_FAIL", "1")

	job, err := env.sj.EnqueueInstall(ctx, "main", "")
	if err != nil {
		t.Fatalf("EnqueueInstall: %v", err)
	}
	final, err := env.runner.WaitFor(ctx, job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobFailed {
		t.Fatalf("expected failed, got %v", final.Status)
	}
	if final.Error == "" {
		t.Error("expected a non-empty job error")
	}

	inst, err := env.svc.Get(ctx, "main")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if inst.InstalledBuildID != "" {
		t.Errorf("expected installed_buildid to remain empty after a failed install, got %q", inst.InstalledBuildID)
	}
}

func TestSteamJobs_EnqueueInstall_Busy(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	ctx := context.Background()
	if _, err := env.svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}

	job1, err := env.sj.EnqueueInstall(ctx, "main", "a")
	if err != nil {
		t.Fatalf("first EnqueueInstall: %v", err)
	}
	_, err = env.sj.EnqueueInstall(ctx, "main", "b")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeInstanceBusy {
		t.Errorf("expected instance_busy, got %v", de.Code)
	}
	if _, err := env.runner.WaitFor(ctx, job1.ID); err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
}

func TestSteamJobs_EnqueueInstall_Running(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	ctx := context.Background()
	if _, err := env.svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	env.sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning, PID: 1})

	_, err := env.sj.EnqueueInstall(ctx, "main", "a")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeInstanceRunning {
		t.Errorf("expected instance_running, got %v", de.Code)
	}
}

func TestSteamJobs_EnqueueInstall_SteamCMDMissing(t *testing.T) {
	svc, sj := newSteamCMDMissingEnv(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := sj.EnqueueInstall(ctx, "main", "a")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeSteamCMDMissing {
		t.Errorf("expected steamcmd_missing, got %v", de.Code)
	}
}

func TestSteamJobs_EnqueueInstall_NotFound(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	_, err := env.sj.EnqueueInstall(context.Background(), "missing", "")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeNotFound {
		t.Errorf("expected not_found, got %v", de.Code)
	}
}

// ---------------------------------------------------------------- update

func TestSteamJobs_EnqueueUpdate_NotRunning(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	ctx := context.Background()
	if _, err := env.svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, env.svc, "main")
	if err := env.svc.SetInstalledBuildID(ctx, "main", "10000001"); err != nil {
		t.Fatalf("SetInstalledBuildID: %v", err)
	}
	t.Setenv("FAKE_STEAMCMD_BUILDID", "10000002")

	job, err := env.sj.EnqueueUpdate(ctx, "main", "op", false)
	if err != nil {
		t.Fatalf("EnqueueUpdate: %v", err)
	}
	final, err := env.runner.WaitFor(ctx, job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected succeeded, got %v (err=%q)", final.Status, final.Error)
	}
	if final.Summary["restarted"] != false {
		t.Errorf("expected restarted=false, got %+v", final.Summary)
	}
	if final.Summary["old_buildid"] != "10000001" || final.Summary["new_buildid"] != "10000002" {
		t.Errorf("unexpected summary: %+v", final.Summary)
	}
	if len(env.sup.startCalls) != 0 {
		t.Errorf("did not expect Start to be called, got %v", env.sup.startCalls)
	}
}

func TestSteamJobs_EnqueueUpdate_RunningNoStop(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	ctx := context.Background()
	if _, err := env.svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, env.svc, "main")
	env.sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning, PID: 1})

	job, err := env.sj.EnqueueUpdate(ctx, "main", "op", false)
	if err != nil {
		t.Fatalf("EnqueueUpdate: %v", err)
	}
	final, err := env.runner.WaitFor(ctx, job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobFailed {
		t.Fatalf("expected failed, got %v", final.Status)
	}
	if !strings.Contains(final.Error, "running") {
		t.Errorf("expected a running-related error, got %q", final.Error)
	}
	if len(env.sup.stopCalls) != 0 {
		t.Errorf("did not expect Stop to be called, got %v", env.sup.stopCalls)
	}
}

func TestSteamJobs_EnqueueUpdate_StopAndStart(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	ctx := context.Background()
	if _, err := env.svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, env.svc, "main")
	env.sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning, PID: 1})
	t.Setenv("FAKE_STEAMCMD_BUILDID", "20000001")

	job, err := env.sj.EnqueueUpdate(ctx, "main", "op", true)
	if err != nil {
		t.Fatalf("EnqueueUpdate: %v", err)
	}
	final, err := env.runner.WaitFor(ctx, job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected succeeded, got %v (err=%q)", final.Status, final.Error)
	}
	if final.Summary["restarted"] != true {
		t.Errorf("expected restarted=true, got %+v", final.Summary)
	}
	if len(env.sup.stopCalls) == 0 {
		t.Error("expected Stop to be called")
	}
	if len(env.sup.startCalls) == 0 {
		t.Error("expected Start to be called")
	}
}

func TestSteamJobs_EnqueueUpdate_Busy(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	ctx := context.Background()
	if _, err := env.svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, env.svc, "main")

	job1, err := env.sj.EnqueueUpdate(ctx, "main", "a", false)
	if err != nil {
		t.Fatalf("first EnqueueUpdate: %v", err)
	}
	_, err = env.sj.EnqueueUpdate(ctx, "main", "b", false)
	de := requireDomainError(t, err)
	if de.Code != domain.CodeInstanceBusy {
		t.Errorf("expected instance_busy, got %v", de.Code)
	}
	if _, err := env.runner.WaitFor(ctx, job1.ID); err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
}

func TestSteamJobs_EnqueueUpdate_SteamCMDMissing(t *testing.T) {
	svc, sj := newSteamCMDMissingEnv(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := sj.EnqueueUpdate(ctx, "main", "a", false)
	de := requireDomainError(t, err)
	if de.Code != domain.CodeSteamCMDMissing {
		t.Errorf("expected steamcmd_missing, got %v", de.Code)
	}
}

func TestSteamJobs_EnqueueUpdate_NotFound(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	_, err := env.sj.EnqueueUpdate(context.Background(), "missing", "", false)
	de := requireDomainError(t, err)
	if de.Code != domain.CodeNotFound {
		t.Errorf("expected not_found, got %v", de.Code)
	}
}

// ---------------------------------------------------------------- pre-update backup

func TestSteamJobs_EnqueueUpdate_PreUpdateBackup_Invoked(t *testing.T) {
	var mu sync.Mutex
	var called bool
	var gotID string
	pre := func(_ context.Context, instanceID string, log *jobs.Logger) error {
		mu.Lock()
		called = true
		gotID = instanceID
		mu.Unlock()
		log.Printf("fake backup ok")
		return nil
	}
	env := newJobsTestEnv(t, pre)
	ctx := context.Background()
	cfg := validConfig(2456)
	cfg.BackupBeforeUpdate = true
	if _, err := env.svc.Create(ctx, "main", "Main", cfg, false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, env.svc, "main")
	t.Setenv("FAKE_STEAMCMD_BUILDID", "60000001")

	job, err := env.sj.EnqueueUpdate(ctx, "main", "op", false)
	if err != nil {
		t.Fatalf("EnqueueUpdate: %v", err)
	}
	final, err := env.runner.WaitFor(ctx, job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected succeeded, got %v (err=%q)", final.Status, final.Error)
	}
	mu.Lock()
	defer mu.Unlock()
	if !called {
		t.Error("expected the pre-update backup hook to be invoked")
	}
	if gotID != "main" {
		t.Errorf("expected backup hook called with instance id 'main', got %q", gotID)
	}
}

func TestSteamJobs_EnqueueUpdate_PreUpdateBackup_Aborts(t *testing.T) {
	backupErr := errors.New("disk full")
	pre := func(_ context.Context, _ string, _ *jobs.Logger) error { return backupErr }
	env := newJobsTestEnv(t, pre)
	ctx := context.Background()
	cfg := validConfig(2456)
	cfg.BackupBeforeUpdate = true
	if _, err := env.svc.Create(ctx, "main", "Main", cfg, false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, env.svc, "main")
	if err := env.svc.SetInstalledBuildID(ctx, "main", "70000001"); err != nil {
		t.Fatalf("SetInstalledBuildID: %v", err)
	}
	t.Setenv("FAKE_STEAMCMD_BUILDID", "70000002")

	job, err := env.sj.EnqueueUpdate(ctx, "main", "op", false)
	if err != nil {
		t.Fatalf("EnqueueUpdate: %v", err)
	}
	final, err := env.runner.WaitFor(ctx, job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobFailed {
		t.Fatalf("expected failed, got %v", final.Status)
	}
	if !strings.Contains(final.Error, "disk full") {
		t.Errorf("expected backup error to surface in job error, got %q", final.Error)
	}

	inst, err := env.svc.Get(ctx, "main")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if inst.InstalledBuildID != "70000001" {
		t.Errorf("expected installed_buildid unchanged after an aborted update, got %q", inst.InstalledBuildID)
	}
}

func TestSteamJobs_EnqueueUpdate_PreUpdateBackup_SkippedWhenDisabled(t *testing.T) {
	var called bool
	pre := func(_ context.Context, _ string, _ *jobs.Logger) error {
		called = true
		return nil
	}
	env := newJobsTestEnv(t, pre)
	ctx := context.Background()
	cfg := validConfig(2456)
	cfg.BackupBeforeUpdate = false
	if _, err := env.svc.Create(ctx, "main", "Main", cfg, false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, env.svc, "main")
	t.Setenv("FAKE_STEAMCMD_BUILDID", "80000001")

	job, err := env.sj.EnqueueUpdate(ctx, "main", "op", false)
	if err != nil {
		t.Fatalf("EnqueueUpdate: %v", err)
	}
	final, err := env.runner.WaitFor(ctx, job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected succeeded, got %v (err=%q)", final.Status, final.Error)
	}
	if called {
		t.Error("expected the pre-update backup hook to be skipped when backup_before_update is false")
	}
}

// ---------------------------------------------------------------- update check

func writeAppManifest(t *testing.T, paths domain.InstancePaths, buildID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(paths.AppManifest()), 0o755); err != nil {
		t.Fatalf("mkdir steamapps: %v", err)
	}
	manifest := "\"AppState\"\n{\n\t\"appid\"\t\t\"896660\"\n\t\"buildid\"\t\t\"" + buildID + "\"\n}\n"
	if err := os.WriteFile(paths.AppManifest(), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write appmanifest: %v", err)
	}
}

func TestSteamJobs_CheckUpdate_NoChecker(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	ctx := context.Background()
	if _, err := env.svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, env.svc, "main")
	writeAppManifest(t, env.svc.Paths("main"), "10000001")
	t.Setenv("FAKE_STEAMCMD_BUILDID", "10000002")

	info, err := env.sj.CheckUpdate(ctx, "main")
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if info.InstanceID != "main" || info.InstalledBuildID != "10000001" || info.LatestBuildID != "10000002" {
		t.Errorf("unexpected info: %+v", info)
	}
	if !info.UpdateAvailable {
		t.Error("expected update_available=true")
	}
	if info.CheckedAt == nil {
		t.Error("expected CheckedAt to be set")
	}

	inst, err := env.svc.Get(ctx, "main")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if inst.InstalledBuildID != "10000001" || inst.LatestBuildID != "10000002" {
		t.Errorf("expected persisted build ids, got installed=%q latest=%q", inst.InstalledBuildID, inst.LatestBuildID)
	}
	if inst.BuildIDCheckedAt == nil {
		t.Error("expected buildid_checked_at to be persisted")
	}
	if !inst.Status.UpdateAvailable {
		t.Error("expected composed status to report update_available")
	}
}

func TestSteamJobs_CheckUpdate_UpToDate(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	ctx := context.Background()
	if _, err := env.svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, env.svc, "main")
	writeAppManifest(t, env.svc.Paths("main"), "99999999")
	t.Setenv("FAKE_STEAMCMD_BUILDID", "99999999")

	info, err := env.sj.CheckUpdate(ctx, "main")
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if info.UpdateAvailable {
		t.Error("expected update_available=false when installed == latest")
	}
}

func TestSteamJobs_CheckUpdate_WithChecker(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	ctx := context.Background()
	if _, err := env.svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, env.svc, "main")
	t.Setenv("FAKE_STEAMCMD_BUILDID", "40000001")

	listInstalled := func(ctx context.Context) ([]steam.InstalledRef, error) {
		return []steam.InstalledRef{{ID: "main", InstallDir: env.svc.Paths("main").Server}}, nil
	}
	store := func(ctx context.Context, id, installed, latest string, at time.Time) error {
		return env.svc.SetBuildIDs(ctx, id, installed, latest, at)
	}
	checker := steam.NewUpdateChecker(env.client, func() time.Duration { return time.Hour }, listInstalled, store, env.bus)
	sj := NewSteamJobs(env.svc, env.runner, env.client, checker, nil, env.log)

	info, err := sj.CheckUpdate(ctx, "main")
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if info.LatestBuildID != "40000001" {
		t.Errorf("expected latest_buildid 40000001, got %q", info.LatestBuildID)
	}
	if info.InstalledBuildID != "" {
		t.Errorf("expected empty installed_buildid (no local manifest), got %q", info.InstalledBuildID)
	}
	if info.UpdateAvailable {
		t.Error("expected update_available=false when installed build id is unknown")
	}
}

func TestSteamJobs_CheckUpdate_NotFound(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	_, err := env.sj.CheckUpdate(context.Background(), "missing")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeNotFound {
		t.Errorf("expected not_found, got %v", de.Code)
	}
}

func TestSteamJobs_CheckUpdate_SteamCMDMissing(t *testing.T) {
	svc, sj := newSteamCMDMissingEnv(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := sj.CheckUpdate(ctx, "main")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeSteamCMDMissing {
		t.Errorf("expected steamcmd_missing, got %v", de.Code)
	}
}

// ---------------------------------------------------------------- misc accessors

func TestSteamJobs_SteamCMDInstalled(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	if !env.sj.SteamCMDInstalled() {
		t.Error("expected the fake steamcmd script to be reported installed")
	}

	_, sj := newSteamCMDMissingEnv(t)
	if sj.SteamCMDInstalled() {
		t.Error("expected a missing steamcmd path to be reported not installed")
	}
}

func TestSteamJobs_LatestBuildID(t *testing.T) {
	env := newJobsTestEnv(t, nil)
	if build, info := env.sj.LatestBuildID(); build != "" || info != nil {
		t.Errorf("expected no cached latest build before any check, got %q %+v", build, info)
	}

	ctx := context.Background()
	if _, err := env.svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, env.svc, "main")
	t.Setenv("FAKE_STEAMCMD_BUILDID", "50000001")
	if _, err := env.sj.CheckUpdate(ctx, "main"); err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}

	build, info := env.sj.LatestBuildID()
	if build != "50000001" {
		t.Errorf("expected latest build 50000001, got %q", build)
	}
	if info == nil || info.LatestBuildID != "50000001" || info.CheckedAt == nil {
		t.Errorf("unexpected info: %+v", info)
	}
}

// ---------------------------------------------------------------- real supervisor

// selfBinOnce/selfBinPath cache a real valheim-ui binary built once for the
// whole test process (only paid by tests that actually exercise the direct
// supervisor), mirroring internal/supervisor's own TestMain.
var (
	selfBinOnce sync.Once
	selfBinPath string
	selfBinErr  error
)

func buildSelfBinary(t *testing.T) string {
	t.Helper()
	selfBinOnce.Do(func() {
		tmp, err := os.MkdirTemp("", "vsui-instance-test-bin-*")
		if err != nil {
			selfBinErr = err
			return
		}
		path := filepath.Join(tmp, "vsui-test-bin")
		cmd := exec.Command("go", "build", "-o", path, "./cmd/valheim-ui")
		cmd.Dir = repoRootForJobsTest()
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			selfBinErr = errWithOutput(err, out)
			return
		}
		selfBinPath = path
	})
	if selfBinErr != nil {
		t.Fatalf("build self binary: %v", selfBinErr)
	}
	return selfBinPath
}

func errWithOutput(err error, out []byte) error {
	return errors.New(err.Error() + ": " + string(out))
}

func repoRootForJobsTest() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

func waitForInstanceState(t *testing.T, svc *Service, id string, want domain.InstanceState, timeout time.Duration) domain.InstanceStatus {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last domain.InstanceStatus
	for time.Now().Before(deadline) {
		st, err := svc.Status(context.Background(), id)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		last = *st
		if st.State == want {
			return last
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for state %v, last=%+v", want, last)
	return last
}

// TestSteamJobs_EnqueueUpdate_RealSupervisor_StopStart exercises the update
// job end-to-end against the real "direct" supervisor and
// testdata/fake-server.sh (docs/WORKPLAN.md WP-05: "update flow restarts the
// fake server afterwards"), instead of the in-memory fakeSupervisor used by
// the other tests in this file.
func TestSteamJobs_EnqueueUpdate_RealSupervisor_StopStart(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a real subprocess")
	}
	bin := buildSelfBinary(t)

	ctx := context.Background()
	sqldb, err := db.OpenMemory(ctx)
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	dataDir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dataDir
	cfg.Supervisor = "direct"

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := &fakeBus{}
	sup := supervisor.NewDirect(supervisor.Options{SelfPath: bin, InstancesDir: cfg.InstancesDir(), Log: log})
	svc := New(sqldb, bus, sup, cfg, log)

	fakeServer, err := filepath.Abs(filepath.Join("..", "..", "testdata", "fake-server.sh"))
	if err != nil {
		t.Fatalf("resolve fake-server.sh: %v", err)
	}
	cfgFile := filepath.Join(dataDir, "config.yaml")
	cfgYAML := "data_dir: " + strconv.Quote(dataDir) + "\n" +
		"supervisor: direct\n" +
		"fake_server: true\n" +
		"fake_server_path: " + strconv.Quote(fakeServer) + "\n"
	if err := os.WriteFile(cfgFile, []byte(cfgYAML), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	t.Setenv("VALHEIM_UI_CONFIG", cfgFile)

	runner := jobs.New(sqldb, bus, filepath.Join(dataDir, "jobs"), log)
	client := steam.New(fakeSteamCMDPath(t), log)
	sj := NewSteamJobs(svc, runner, client, nil, nil, log)

	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, svc, "main")
	if err := svc.SetInstalledBuildID(ctx, "main", "30000001"); err != nil {
		t.Fatalf("SetInstalledBuildID: %v", err)
	}

	if _, err := svc.Start(ctx, "main"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = svc.Stop(context.Background(), "main") })
	waitForInstanceState(t, svc, "main", domain.StateRunning, 10*time.Second)

	t.Setenv("FAKE_STEAMCMD_BUILDID", "30000002")
	job, err := sj.EnqueueUpdate(ctx, "main", "op", true)
	if err != nil {
		t.Fatalf("EnqueueUpdate: %v", err)
	}
	final, err := runner.WaitFor(ctx, job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected succeeded, got %v (err=%q)", final.Status, final.Error)
	}
	if final.Summary["restarted"] != true {
		t.Errorf("expected restarted=true, got %+v", final.Summary)
	}
	if final.Summary["old_buildid"] != "30000001" || final.Summary["new_buildid"] != "30000002" {
		t.Errorf("unexpected summary: %+v", final.Summary)
	}

	waitForInstanceState(t, svc, "main", domain.StateRunning, 10*time.Second)

	inst, err := svc.Get(ctx, "main")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if inst.InstalledBuildID != "30000002" {
		t.Errorf("expected installed_buildid 30000002 after update, got %q", inst.InstalledBuildID)
	}
}

// TestSteamJobs_EnqueueUpdate_RestartsAfterFailure covers the reliability fix:
// a previously-running instance must be brought back even when the update
// fails after it has been stopped (here the pre-update backup errors). The old
// install is intact, so leaving the server offline would be the worse outcome.
func TestSteamJobs_EnqueueUpdate_RestartsAfterFailure(t *testing.T) {
	env := newJobsTestEnv(t, func(context.Context, string, *jobs.Logger) error {
		return fmt.Errorf("simulated backup failure")
	})
	ctx := context.Background()
	cfg := validConfig(2456)
	cfg.BackupBeforeUpdate = true
	if _, err := env.svc.Create(ctx, "main", "Main", cfg, false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, env.svc, "main")
	env.sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning, PID: 1})

	job, err := env.sj.EnqueueUpdate(ctx, "main", "op", true)
	if err != nil {
		t.Fatalf("EnqueueUpdate: %v", err)
	}
	final, err := env.runner.WaitFor(ctx, job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobFailed {
		t.Fatalf("expected the update to fail, got %v", final.Status)
	}
	// It was stopped for the update, and restarted on the way out despite the failure.
	if len(env.sup.stopCalls) == 0 {
		t.Fatal("instance should have been stopped for the update")
	}
	if len(env.sup.startCalls) == 0 {
		t.Fatal("instance must be restarted after a failed update, not left offline")
	}
	if st, _ := env.svc.Status(ctx, "main"); st.State != domain.StateRunning {
		t.Fatalf("instance should be running again after the failed update, got %s", st.State)
	}
}
