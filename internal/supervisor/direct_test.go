package supervisor

import (
	"context"
	"encoding/json"
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

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// selfBinPath is a real valheim-ui binary built once for the whole package,
// so the direct supervisor can spawn `<selfBinPath> launch --instance <id>`
// exactly like it does in production.
var selfBinPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "vsui-supervisor-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	selfBinPath = filepath.Join(tmp, "vsui-test-bin")
	cmd := exec.Command("go", "build", "-o", selfBinPath, "./cmd/valheim-ui")
	cmd.Dir = repoRoot()
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		println("build test binary failed:\n" + string(out))
		panic(err)
	}

	os.Exit(m.Run())
}

func repoRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// setupDirectInstance writes a launch.json and a config.yaml pointing the
// launcher at testdata/fake-server.sh, sets VALHEIM_UI_CONFIG for the
// (inherited-env) child processes the direct supervisor spawns, and returns a
// ready-to-use Supervisor.
func setupDirectInstance(t *testing.T) (sup Supervisor, id string, consoleLog string) {
	t.Helper()
	dir := t.TempDir()
	instancesDir := filepath.Join(dir, "instances")
	id = "main"
	paths := domain.PathsFor(instancesDir, id)
	for _, d := range []string{paths.Server, paths.Logs, paths.Save} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	launch := domain.Launch{
		Version:    domain.LaunchVersion,
		InstanceID: id,
		ServerDir:  paths.Server,
		LogDir:     paths.Logs,
		Args:       []string{"-name", "Test", "-port", "2456", "-world", "Dedicated", "-password", "secret12"},
	}
	data, err := json.Marshal(launch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.LaunchFile(), data, 0o600); err != nil {
		t.Fatal(err)
	}

	fakeServer := filepath.Join(repoRoot(), "testdata", "fake-server.sh")
	cfgFile := filepath.Join(dir, "config.yaml")
	cfgYAML := "data_dir: " + strconv.Quote(dir) + "\n" +
		"supervisor: direct\n" +
		"fake_server: true\n" +
		"fake_server_path: " + strconv.Quote(fakeServer) + "\n"
	if err := os.WriteFile(cfgFile, []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VALHEIM_UI_CONFIG", cfgFile)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sup = NewDirect(Options{SelfPath: selfBinPath, InstancesDir: instancesDir, Log: log})
	return sup, id, paths.ConsoleLog()
}

// waitForLogLine polls consoleLog for substr. The direct supervisor marks an
// instance "running" as soon as the child process has forked (there is no
// readiness probe at this layer, ARCHITECTURE.md §6), so tests that then send
// it a signal must first wait for the fake server to actually be up and past
// its `trap` registration, or the signal can race the exec chain and the
// process dies from the *default* SIGINT action instead of the trap.
func waitForLogLine(t *testing.T, consoleLog, substr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(consoleLog); err == nil && strings.Contains(string(b), substr) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in %s", substr, consoleLog)
}

func waitForState(t *testing.T, sup Supervisor, id string, want State, timeout time.Duration) Status {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last Status
	for time.Now().Before(deadline) {
		st, err := sup.Status(context.Background(), id)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		last = st
		if st.State == want {
			return st
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for state %v, last status: %+v", want, last)
	return last
}

func TestDirect_StartStatusStop(t *testing.T) {
	sup, id, consoleLog := setupDirectInstance(t)
	ctx := context.Background()

	if err := sup.Start(ctx, id); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st := waitForState(t, sup, id, StateRunning, 10*time.Second)
	if st.PID == 0 {
		t.Errorf("expected a non-zero PID while running, got %+v", st)
	}
	if st.Since.IsZero() {
		t.Errorf("expected Since to be set while running")
	}
	waitForLogLine(t, consoleLog, "Game server connected", 10*time.Second)

	// Start again while running is a no-op.
	if err := sup.Start(ctx, id); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	if err := sup.Stop(ctx, id); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	st = waitForState(t, sup, id, StateStopped, 10*time.Second)
	if st.PID != 0 {
		t.Errorf("expected PID 0 once stopped, got %d", st.PID)
	}

	// Stop again once stopped is a no-op.
	if err := sup.Stop(ctx, id); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestDirect_Restart(t *testing.T) {
	sup, id, consoleLog := setupDirectInstance(t)
	ctx := context.Background()

	if err := sup.Start(ctx, id); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForState(t, sup, id, StateRunning, 10*time.Second)
	waitForLogLine(t, consoleLog, "Game server connected", 10*time.Second)
	first, _ := sup.Status(ctx, id)

	if err := sup.Restart(ctx, id); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	second := waitForState(t, sup, id, StateRunning, 10*time.Second)
	if second.PID == first.PID {
		t.Errorf("expected a different PID after restart (first=%d)", first.PID)
	}
	_ = sup.Stop(ctx, id)
}

func TestDirect_StopNeverStarted(t *testing.T) {
	sup, id, _ := setupDirectInstance(t)
	ctx := context.Background()
	if err := sup.Stop(ctx, id); err != nil {
		t.Fatalf("Stop on never-started instance should be a no-op: %v", err)
	}
	st, err := sup.Status(ctx, id)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.State != StateStopped {
		t.Errorf("expected stopped, got %v", st.State)
	}
}

func TestDirect_SetAutostart(t *testing.T) {
	sup, id, _ := setupDirectInstance(t)
	ctx := context.Background()
	if err := sup.SetAutostart(ctx, id, true); err != nil {
		t.Fatalf("SetAutostart: %v", err)
	}
	st, err := sup.Status(ctx, id)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Autostart {
		t.Errorf("expected Autostart=true after SetAutostart(true)")
	}
	if err := sup.SetAutostart(ctx, id, false); err != nil {
		t.Fatalf("SetAutostart: %v", err)
	}
	st, _ = sup.Status(ctx, id)
	if st.Autostart {
		t.Errorf("expected Autostart=false after SetAutostart(false)")
	}
}

func TestDirect_ConcurrentUse(t *testing.T) {
	sup, id, consoleLog := setupDirectInstance(t)
	ctx := context.Background()
	if err := sup.Start(ctx, id); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForState(t, sup, id, StateRunning, 10*time.Second)
	waitForLogLine(t, consoleLog, "Game server connected", 10*time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = sup.Status(ctx, id)
			_ = sup.SetAutostart(ctx, id, true)
		}()
	}
	wg.Wait()
	_ = sup.Stop(ctx, id)
}
