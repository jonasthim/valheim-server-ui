package instance

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

func newTestService(t *testing.T) (*Service, *fakeSupervisor, *fakeBus) {
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

	sup := newFakeSupervisor()
	bus := &fakeBus{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(sqldb, bus, sup, cfg, log), sup, bus
}

func validConfig(port int) domain.InstanceConfig {
	c := domain.InstanceConfig{
		Name:     "My Server",
		World:    "Dedicated",
		Password: "secret123",
		Port:     port,
		Public:   true,
	}
	c.ApplyDefaults()
	return c
}

func markInstalled(t *testing.T, svc *Service, id string) {
	t.Helper()
	bin := svc.Paths(id).ServerBinary()
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCreate_Success(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	inst, err := svc.Create(ctx, "main", "Main", validConfig(2456), true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.ID != "main" || inst.Name != "Main" {
		t.Errorf("unexpected instance: %+v", inst)
	}
	if inst.Status.State != domain.StateNotInstalled {
		t.Errorf("expected not_installed state, got %v", inst.Status.State)
	}

	paths := svc.Paths("main")
	for _, dir := range []string{paths.Root, paths.Server, paths.Save, paths.WorldsDir(), paths.Backups, paths.Logs} {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			t.Errorf("expected directory %s to exist: %v", dir, err)
		}
	}

	data, err := os.ReadFile(paths.LaunchFile())
	if err != nil {
		t.Fatalf("read launch.json: %v", err)
	}
	var launch domain.Launch
	if err := json.Unmarshal(data, &launch); err != nil {
		t.Fatalf("parse launch.json: %v", err)
	}
	if launch.ServerDir != paths.Server || launch.LogDir != paths.Logs {
		t.Errorf("unexpected launch.json paths: %+v", launch)
	}
	if len(launch.Args) == 0 || launch.Args[0] != "-name" {
		t.Errorf("unexpected launch.json args: %v", launch.Args)
	}

	got, err := svc.Get(ctx, "main")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Config.Port != 2456 {
		t.Errorf("Get returned wrong config: %+v", got.Config)
	}
}

func TestCreate_InvalidID(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Create(context.Background(), "Bad_ID!", "Name", validConfig(2456), false)
	de := requireDomainError(t, err)
	if de.Code != domain.CodeValidationFailed {
		t.Errorf("expected validation_failed, got %v", de.Code)
	}
}

func TestCreate_InvalidConfig(t *testing.T) {
	svc, _, _ := newTestService(t)
	cfg := validConfig(2456)
	cfg.Password = "no" // too short
	_, err := svc.Create(context.Background(), "main", "Main", cfg, false)
	de := requireDomainError(t, err)
	if de.Code != domain.CodeValidationFailed {
		t.Errorf("expected validation_failed, got %v", de.Code)
	}
}

func TestCreate_Duplicate(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(ctx, "main", "Main2", validConfig(2500), false)
	de := requireDomainError(t, err)
	if de.Code != domain.CodeConflict {
		t.Errorf("expected conflict, got %v", de.Code)
	}
}

func TestCreate_PortOverlap(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "a", "A", validConfig(2456), false); err != nil {
		t.Fatalf("Create a: %v", err)
	}

	// 2456 occupies 2456-2458; 2458 overlaps, 2459 does not.
	if _, err := svc.Create(ctx, "b", "B", validConfig(2458), false); err == nil {
		t.Fatal("expected a port_in_use error")
	} else if de := requireDomainError(t, err); de.Code != domain.CodePortInUse {
		t.Errorf("expected port_in_use, got %v", de.Code)
	}

	if _, err := svc.Create(ctx, "c", "C", validConfig(2459), false); err != nil {
		t.Errorf("adjacent non-overlapping port should be accepted: %v", err)
	}
}

func TestUpdate_PendingRestartWhenRunning(t *testing.T) {
	svc, sup, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning, PID: 1})

	newCfg := validConfig(2456)
	newCfg.Name = "Renamed In-Game"
	inst, err := svc.Update(ctx, "main", nil, &newCfg, nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !inst.PendingRestart {
		t.Errorf("expected pending_restart=true after a config change while running")
	}

	data, _ := os.ReadFile(svc.Paths("main").LaunchFile())
	var launch domain.Launch
	_ = json.Unmarshal(data, &launch)
	found := false
	for _, a := range launch.Args {
		if a == "Renamed In-Game" {
			found = true
		}
	}
	if !found {
		t.Errorf("launch.json was not re-rendered with the new config: %v", launch.Args)
	}
}

func TestUpdate_NoPendingRestartWhenStopped(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	newCfg := validConfig(2456)
	newCfg.Name = "Changed"
	inst, err := svc.Update(ctx, "main", nil, &newCfg, nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if inst.PendingRestart {
		t.Errorf("did not expect pending_restart while stopped")
	}
}

func TestUpdate_AutostartTogglesSupervisor(t *testing.T) {
	svc, sup, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	on := true
	if _, err := svc.Update(ctx, "main", nil, nil, &on); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !sup.autostart["main"] {
		t.Errorf("expected supervisor.SetAutostart(true) to have been called")
	}
}

func TestUpdate_PortOverlapExcludesSelf(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	cfg := validConfig(2456)
	cfg.Public = false
	if _, err := svc.Update(ctx, "main", nil, &cfg, nil); err != nil {
		t.Errorf("updating with its own unchanged port should not conflict: %v", err)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Update(context.Background(), "missing", nil, nil, nil)
	de := requireDomainError(t, err)
	if de.Code != domain.CodeNotFound {
		t.Errorf("expected not_found, got %v", de.Code)
	}
}

func TestDelete_RefusesWhileRunning(t *testing.T) {
	svc, sup, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning})
	err := svc.Delete(ctx, "main", false)
	de := requireDomainError(t, err)
	if de.Code != domain.CodeInstanceRunning {
		t.Errorf("expected instance_running, got %v", de.Code)
	}
}

func TestDelete_KeepsFilesByDefault(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	root := svc.Paths("main").Root
	if err := svc.Delete(ctx, "main", false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Errorf("expected instance directory to remain when delete_files=false: %v", err)
	}
	if _, err := svc.Get(ctx, "main"); err == nil {
		t.Errorf("expected the instance row to be gone")
	}
}

func TestDelete_RemovesFilesWhenRequested(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	root := svc.Paths("main").Root
	if err := svc.Delete(ctx, "main", true); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("expected instance directory to be removed, stat err = %v", err)
	}
}

func TestStart_NotInstalled(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := svc.Start(ctx, "main")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeInstanceNotInstalled {
		t.Errorf("expected instance_not_installed, got %v", de.Code)
	}
}

func TestStart_Success(t *testing.T) {
	svc, sup, bus := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, svc, "main")

	// Force pending_restart so we can assert Start clears it.
	if err := svc.MarkPendingRestart(ctx, "main"); err != nil {
		t.Fatalf("MarkPendingRestart: %v", err)
	}

	st, err := svc.Start(ctx, "main")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if st.State != domain.StateRunning {
		t.Errorf("expected running, got %v", st.State)
	}
	if st.PendingRestart {
		t.Errorf("expected pending_restart cleared after Start")
	}
	if len(sup.startCalls) != 1 || sup.startCalls[0] != "main" {
		t.Errorf("expected supervisor.Start(main), got %v", sup.startCalls)
	}
	events := bus.all()
	if len(events) == 0 || events[len(events)-1].Name != domain.EventInstanceStatus {
		t.Errorf("expected an instance.status event to be published, got %+v", events)
	}
}

func TestStop_And_Restart(t *testing.T) {
	svc, sup, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	markInstalled(t, svc, "main")
	if _, err := svc.Start(ctx, "main"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, err := svc.Stop(ctx, "main"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(sup.stopCalls) != 1 {
		t.Errorf("expected one Stop call, got %v", sup.stopCalls)
	}

	if err := svc.MarkPendingRestart(ctx, "main"); err != nil {
		t.Fatalf("MarkPendingRestart: %v", err)
	}
	st, err := svc.Restart(ctx, "main")
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if st.PendingRestart {
		t.Errorf("expected pending_restart cleared after Restart")
	}
	if len(sup.restartCalls) != 1 {
		t.Errorf("expected one Restart call, got %v", sup.restartCalls)
	}
}

func TestStatus_Enricher(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	svc.RegisterEnricher(domain.StatusEnricherFunc(func(_ context.Context, st *domain.InstanceStatus) {
		st.PlayersOnline = 3
		st.JoinCode = "999999"
	}))
	st, err := svc.Status(ctx, "main")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.PlayersOnline != 3 || st.JoinCode != "999999" {
		t.Errorf("enricher was not applied: %+v", st)
	}
}

func TestSetInstalledBuildID(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.SetInstalledBuildID(ctx, "main", "12345"); err != nil {
		t.Fatalf("SetInstalledBuildID: %v", err)
	}
	st, err := svc.Status(ctx, "main")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.InstalledBuildID != "12345" {
		t.Errorf("expected installed_buildid=12345, got %q", st.InstalledBuildID)
	}
}

func TestTailLog(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}

	logPath := svc.Paths("main").ConsoleLog()
	var content string
	for i := 1; i <= 50; i++ {
		content += "line " + strconv.Itoa(i) + "\n"
	}
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	lines, err := svc.TailLog(ctx, "main", 10)
	if err != nil {
		t.Fatalf("TailLog: %v", err)
	}
	if len(lines) != 10 || lines[0] != "line 41" || lines[9] != "line 50" {
		t.Errorf("unexpected tail: %v", lines)
	}

	all, err := svc.TailLog(ctx, "main", 1000)
	if err != nil {
		t.Fatalf("TailLog: %v", err)
	}
	if len(all) != 50 {
		t.Errorf("expected all 50 lines when n > total, got %d", len(all))
	}

	def, err := svc.TailLog(ctx, "main", 0)
	if err != nil {
		t.Fatalf("TailLog default: %v", err)
	}
	if len(def) != 50 {
		t.Errorf("expected default lines to return everything available (%d), got %d", 50, len(def))
	}
}

func TestTailLog_MissingFile(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	lines, err := svc.TailLog(ctx, "main", 100)
	if err != nil {
		t.Fatalf("TailLog: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("expected no lines for a missing log file, got %v", lines)
	}
}

func TestTailLog_LargeFileMultiChunk(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	logPath := svc.Paths("main").ConsoleLog()
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	// Pad well past one 64 KiB read chunk before the lines we actually check.
	padding := make([]byte, 200*1024)
	for i := range padding {
		padding[i] = 'x'
	}
	if _, err := f.Write(padding); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		if _, err := f.WriteString("tail-" + strconv.Itoa(i) + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	lines, err := svc.TailLog(ctx, "main", 5)
	if err != nil {
		t.Fatalf("TailLog: %v", err)
	}
	want := []string{"tail-1", "tail-2", "tail-3", "tail-4", "tail-5"}
	if len(lines) != len(want) {
		t.Fatalf("got %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestOpenLog(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	logPath := svc.Paths("main").ConsoleLog()
	if err := os.WriteFile(logPath, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rc, err := svc.OpenLog(ctx, "main")
	if err != nil {
		t.Fatalf("OpenLog: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Errorf("unexpected content: %q", data)
	}
}

func TestPollOnce_PublishesOnStateChange(t *testing.T) {
	svc, sup, bus := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "main", "Main", validConfig(2456), false); err != nil {
		t.Fatalf("Create: %v", err)
	}

	last := map[string]domain.InstanceState{}
	svc.pollOnce(ctx, last)
	if len(bus.all()) != 1 {
		t.Fatalf("expected one publish on first poll, got %d", len(bus.all()))
	}

	svc.pollOnce(ctx, last)
	if len(bus.all()) != 1 {
		t.Fatalf("expected no publish when state is unchanged, got %d", len(bus.all()))
	}

	sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning, PID: 1})
	// installed is still false, but state moves from not_installed to running
	// once the supervisor reports running regardless of the binary check.
	svc.pollOnce(ctx, last)
	if len(bus.all()) != 2 {
		t.Fatalf("expected a publish once the supervisor state changes, got %d", len(bus.all()))
	}
}

func requireDomainError(t *testing.T, err error) *domain.Error {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	de := domain.AsError(err)
	return de
}
