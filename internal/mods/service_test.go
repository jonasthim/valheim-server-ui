package mods

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// newTestModsService wires a real instance.Service (with a fake supervisor,
// per WORKPLAN.md WP-08's test plan) and a real jobs.Runner on an in-memory
// database, plus a mods.Service backed by ts.
func newTestModsService(t *testing.T, ts *Thunderstore) (*Service, *instance.Service, *fakeSupervisor, *jobs.Runner) {
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

	sup := newFakeSupervisor()
	instSvc := instance.New(sqldb, fakeBus{}, sup, cfg, log)
	runner := jobs.New(sqldb, fakeBus{}, cfg.JobsDir(), log)
	modSvc := NewService(sqldb, ts, instSvc, runner, cfg.CacheDir(), log)
	return modSvc, instSvc, sup, runner
}

func validModTestConfig(port int) domain.InstanceConfig {
	c := domain.InstanceConfig{
		Name: "Main", World: "Dedicated", Password: "secret123", Port: port, Public: true,
	}
	c.ApplyDefaults()
	return c
}

func createTestInstance(t *testing.T, svc *instance.Service, id string) {
	t.Helper()
	if _, err := svc.Create(context.Background(), id, "Main", validModTestConfig(2456), false); err != nil {
		t.Fatalf("create instance %s: %v", id, err)
	}
}

// markInstalled makes the instance look installed enough for
// instance.Service.Start to succeed (server binary present).
func markInstalled(t *testing.T, svc *instance.Service, id string) {
	t.Helper()
	bin := svc.Paths(id).ServerBinary()
	if err := writeFileAtomic(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("mark installed: %v", err)
	}
}

func mustSucceed(t *testing.T, runner *jobs.Runner, jobID string) *domain.Job {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	job, err := runner.WaitFor(ctx, jobID)
	if err != nil {
		t.Fatalf("WaitFor(%s): %v", jobID, err)
	}
	if job.Status != domain.JobSucceeded {
		t.Fatalf("job %s ended %s: %s", jobID, job.Status, job.Error)
	}
	return job
}

func TestEnqueueInstall_WithDependenciesProducesExpectedTree(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, _, runner := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")

	job, err := modSvc.EnqueueInstall(ctx, "main", "Alice", "Awesome", "", "tester")
	if err != nil {
		t.Fatalf("EnqueueInstall: %v", err)
	}
	mustSucceed(t, runner, job.ID)

	paths := instSvc.Paths("main")
	assertHasFile(t, paths.Server, "BepInEx/core/BepInEx.Preloader.dll", "preloader")
	assertHasFile(t, paths.Server, "BepInEx/plugins/Alice-CoreLib/CoreLib.dll", "corelib-dll-v1")
	assertHasFile(t, paths.Server, "BepInEx/config/corelib.cfg", "[General]\nEnabled = true\n")
	assertHasFile(t, paths.Server, "BepInEx/plugins/Alice-Awesome/Awesome.dll", "awesome-dll")

	rows, err := modSvc.listModRows(ctx, "main")
	if err != nil {
		t.Fatalf("listModRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 mod rows (CoreLib + Awesome; BepInEx itself is not a mod row), got %d: %+v", len(rows), rows)
	}
	names := map[string]bool{}
	for _, r := range rows {
		names[r.Owner+"-"+r.Name] = true
		if !r.Enabled {
			t.Errorf("expected freshly installed mod %s enabled by default", r.Owner+"-"+r.Name)
		}
	}
	if !names["Alice-CoreLib"] || !names["Alice-Awesome"] {
		t.Errorf("unexpected rows: %v", rows)
	}

	overview, err := modSvc.Overview(ctx, "main")
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if !overview.BepInEx.Installed {
		t.Error("expected BepInEx installed")
	}
	if overview.BepInEx.Version != "5.4.2202" {
		t.Errorf("expected bepinex version 5.4.2202, got %q", overview.BepInEx.Version)
	}
	if len(overview.Mods) != 2 {
		t.Errorf("expected 2 mods in overview, got %d", len(overview.Mods))
	}
}

func TestEnqueueInstall_UnknownPackage404s(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, _, _ := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")

	_, err := modSvc.EnqueueInstall(ctx, "main", "Nobody", "Nothing", "", "tester")
	if err == nil {
		t.Fatal("expected an error")
	}
	if de := domain.AsError(err); de.Code != domain.CodePackageNotFound {
		t.Errorf("expected package_not_found, got %v", de.Code)
	}
}

func TestEnqueueUninstall_RemovesExactlyItsFiles(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, _, runner := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")

	job, err := modSvc.EnqueueInstall(ctx, "main", "Alice", "Awesome", "", "tester")
	if err != nil {
		t.Fatalf("EnqueueInstall: %v", err)
	}
	mustSucceed(t, runner, job.ID)

	rows, err := modSvc.listModRows(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	var coreLibID int64
	for _, r := range rows {
		if r.Name == "CoreLib" {
			coreLibID = r.ID
		}
	}
	if coreLibID == 0 {
		t.Fatal("CoreLib row not found")
	}

	job2, err := modSvc.EnqueueUninstall(ctx, "main", coreLibID, "tester")
	if err != nil {
		t.Fatalf("EnqueueUninstall: %v", err)
	}
	mustSucceed(t, runner, job2.ID)

	paths := instSvc.Paths("main")
	if fileExists(filepath.Join(paths.Server, "BepInEx", "plugins", "Alice-CoreLib")) {
		t.Error("expected CoreLib's plugin dir removed entirely")
	}
	// Its cfg must survive (never deleted), and unrelated mods/BepInEx must be untouched.
	assertHasFile(t, paths.Server, "BepInEx/config/corelib.cfg", "[General]\nEnabled = true\n")
	assertHasFile(t, paths.Server, "BepInEx/plugins/Alice-Awesome/Awesome.dll", "awesome-dll")
	assertHasFile(t, paths.Server, "BepInEx/core/BepInEx.Preloader.dll", "preloader")

	if _, err := modSvc.getModRow(ctx, "main", coreLibID); err == nil {
		t.Error("expected the mod row to be deleted")
	}
}

func TestSetEnabled_TogglesFilesAndPendingRestart(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, sup, runner := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")

	job, err := modSvc.EnqueueInstall(ctx, "main", "Alice", "CoreLib", "", "tester")
	if err != nil {
		t.Fatalf("EnqueueInstall: %v", err)
	}
	mustSucceed(t, runner, job.ID)

	rows, err := modSvc.listModRows(ctx, "main")
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected 1 row, got %+v err=%v", rows, err)
	}
	id := rows[0].ID
	paths := instSvc.Paths("main")

	sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning})

	mod, err := modSvc.SetEnabled(ctx, "main", id, false)
	if err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if mod.Enabled {
		t.Error("expected mod reported disabled")
	}
	if fileExists(filepath.Join(paths.Server, "BepInEx", "plugins", "Alice-CoreLib", "CoreLib.dll")) {
		t.Error("expected the dll renamed away from its enabled path")
	}
	assertHasFile(t, paths.Server, "BepInEx/plugins/Alice-CoreLib/CoreLib.dll.disabled", "corelib-dll-v1")

	inst, err := instSvc.Get(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !inst.PendingRestart {
		t.Error("expected pending_restart after disabling a mod while the instance is running")
	}

	mod, err = modSvc.SetEnabled(ctx, "main", id, true)
	if err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if !mod.Enabled {
		t.Error("expected mod reported enabled")
	}
	assertHasFile(t, paths.Server, "BepInEx/plugins/Alice-CoreLib/CoreLib.dll", "corelib-dll-v1")
}

func TestEnqueueUpdate_KeepsEnabledFlag(t *testing.T) {
	now := time.Now().UTC()
	v1 := rawVersion{
		Name: "CoreLib", FullName: "Alice-CoreLib-1.0.0", VersionNumber: "1.0.0", DateCreated: now,
		DownloadURL: "https://thunderstore.io/package/download/Alice/CoreLib/1.0.0/",
	}
	pkg := rawPackage{Name: "CoreLib", FullName: "Alice-CoreLib", Owner: "Alice", DateCreated: now, DateUpdated: now, Versions: []rawVersion{v1}}
	srv := newTestThunderstoreServer(t, []rawPackage{pkg})
	srv.setZip("Alice", "CoreLib", "1.0.0", coreLibZip(t))

	tsClient := NewThunderstore(srv.client(), t.TempDir(), func() time.Duration { return time.Hour }, "test-agent", nil)
	ctx := context.Background()
	if err := tsClient.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	modSvc, instSvc, _, runner := newTestModsService(t, tsClient)
	createTestInstance(t, instSvc, "main")

	job, err := modSvc.EnqueueInstall(ctx, "main", "Alice", "CoreLib", "", "tester")
	if err != nil {
		t.Fatalf("EnqueueInstall: %v", err)
	}
	mustSucceed(t, runner, job.ID)

	rows, _ := modSvc.listModRows(ctx, "main")
	id := rows[0].ID
	if _, err := modSvc.SetEnabled(ctx, "main", id, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	// Publish an upgrade: 1.1.0 becomes latest, 1.0.0 remains as history.
	v2 := rawVersion{
		Name: "CoreLib", FullName: "Alice-CoreLib-1.1.0", VersionNumber: "1.1.0", DateCreated: now,
		DownloadURL: "https://thunderstore.io/package/download/Alice/CoreLib/1.1.0/",
	}
	pkg.Versions = []rawVersion{v2, v1}
	srv.index = []rawPackage{pkg}
	srv.setZip("Alice", "CoreLib", "1.1.0", coreLibZipV2(t))
	if err := tsClient.Refresh(ctx); err != nil {
		t.Fatalf("refresh 2: %v", err)
	}

	job2, err := modSvc.EnqueueUpdate(ctx, "main", id, "", "tester")
	if err != nil {
		t.Fatalf("EnqueueUpdate: %v", err)
	}
	mustSucceed(t, runner, job2.ID)

	row, err := modSvc.getModRow(ctx, "main", id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Version != "1.1.0" {
		t.Errorf("expected version updated to 1.1.0, got %s", row.Version)
	}
	if row.Enabled {
		t.Error("expected the enabled=false flag to survive the update")
	}

	paths := instSvc.Paths("main")
	assertHasFile(t, paths.Server, "BepInEx/plugins/Alice-CoreLib/CoreLib.dll.disabled", "corelib-dll-v2")
	if fileExists(filepath.Join(paths.Server, "BepInEx", "plugins", "Alice-CoreLib", "CoreLib.dll")) {
		t.Error("expected no enabled-form dll after updating a disabled mod")
	}
}

func TestEnqueueUpdate_RejectsManualMods(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, _, runner := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")

	job, err := modSvc.EnqueueUpload(ctx, "main", "MyMod.dll", bytes.NewReader([]byte("dll-bytes")), "tester")
	if err != nil {
		t.Fatalf("EnqueueUpload: %v", err)
	}
	mustSucceed(t, runner, job.ID)

	rows, _ := modSvc.listModRows(ctx, "main")
	if _, err := modSvc.EnqueueUpdate(ctx, "main", rows[0].ID, "", "tester"); err == nil {
		t.Fatal("expected an error updating a manual mod")
	}
}

func TestEnqueueUpload_DLL(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, _, runner := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")

	job, err := modSvc.EnqueueUpload(ctx, "main", "MyMod.dll", bytes.NewReader([]byte("dll-bytes")), "tester")
	if err != nil {
		t.Fatalf("EnqueueUpload: %v", err)
	}
	mustSucceed(t, runner, job.ID)

	paths := instSvc.Paths("main")
	assertHasFile(t, paths.Server, "BepInEx/plugins/local-mymod/mymod.dll", "dll-bytes")

	rows, err := modSvc.listModRows(ctx, "main")
	if err != nil || len(rows) != 1 || rows[0].Source != domain.ModSourceManual {
		t.Fatalf("unexpected rows: %+v err=%v", rows, err)
	}
}

func TestEnqueueUpload_RejectsUnknownExtension(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, _, _ := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")

	if _, err := modSvc.EnqueueUpload(ctx, "main", "readme.txt", bytes.NewReader([]byte("hi")), "tester"); err == nil {
		t.Fatal("expected a validation error for a non-zip/dll upload")
	}
}

func TestEnqueueBepInExInstall_StopStartFlowAndInstanceRunningGuard(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, sup, runner := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")
	markInstalled(t, instSvc, "main")
	sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning})

	if _, err := modSvc.EnqueueBepInExInstall(ctx, "main", false, "tester"); err == nil {
		t.Fatal("expected instance_running without stop_if_running")
	} else if de := domain.AsError(err); de.Code != domain.CodeInstanceRunning {
		t.Errorf("expected instance_running, got %v", de.Code)
	}

	job, err := modSvc.EnqueueBepInExInstall(ctx, "main", true, "tester")
	if err != nil {
		t.Fatalf("EnqueueBepInExInstall: %v", err)
	}
	mustSucceed(t, runner, job.ID)

	paths := instSvc.Paths("main")
	assertHasFile(t, paths.Server, "BepInEx/core/BepInEx.Preloader.dll", "preloader")

	st, err := instSvc.Status(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != domain.StateRunning {
		t.Errorf("expected the instance restarted after the bepinex install, got %v", st.State)
	}
}

func TestSetBepInExEnabled_RequiresInstalled(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, _, _ := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")

	if _, err := modSvc.SetBepInExEnabled(ctx, "main", true); err == nil {
		t.Fatal("expected bepinex_missing")
	} else if de := domain.AsError(err); de.Code != domain.CodeBepInExMissing {
		t.Errorf("expected bepinex_missing, got %v", de.Code)
	}
}

func TestSetBepInExEnabled_PendingRestartWhileRunning(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, sup, runner := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")
	markInstalled(t, instSvc, "main")

	job, err := modSvc.EnqueueBepInExInstall(ctx, "main", false, "tester")
	if err != nil {
		t.Fatalf("EnqueueBepInExInstall: %v", err)
	}
	mustSucceed(t, runner, job.ID)

	sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning})

	overview, err := modSvc.SetBepInExEnabled(ctx, "main", true)
	if err != nil {
		t.Fatalf("SetBepInExEnabled: %v", err)
	}
	if !overview.BepInEx.Enabled {
		t.Error("expected bepinex reported enabled")
	}
	if !overview.PendingRestart {
		t.Error("expected pending_restart set after enabling bepinex while the instance is running")
	}
}

func TestConfigEditor_UpdateMarksPendingRestartWhileRunning(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, sup, runner := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")

	job, err := modSvc.EnqueueInstall(ctx, "main", "Alice", "CoreLib", "", "tester")
	if err != nil {
		t.Fatalf("EnqueueInstall: %v", err)
	}
	mustSucceed(t, runner, job.ID)

	files, err := modSvc.ListConfigs(ctx, "main")
	if err != nil || len(files) != 1 {
		t.Fatalf("ListConfigs: %+v err=%v", files, err)
	}

	sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning})

	_, err = modSvc.UpdateConfig(ctx, "main", "corelib.cfg", domain.ConfigFileUpdate{
		Values: []domain.ConfigValueUpdate{{Section: "General", Key: "Enabled", Value: "false"}},
	})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	inst, err := instSvc.Get(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !inst.PendingRestart {
		t.Error("expected pending_restart set after a config edit while running")
	}
}
