package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// fakeServerBin is the Go fake game server (tools/fake-server), built once
// per test run so the launcher can be exercised on every platform.
var fakeServerBin string

// TestMain intercepts a re-exec of this test binary (see
// TestRun_ExecsFakeServer) so the *real* exec path (real syscall.Exec, real
// os.Chdir) can be exercised in a throwaway child process instead of the
// process running `go test`.
func TestMain(m *testing.M) {
	if os.Getenv("VSUI_LAUNCHER_HELPER") == "1" {
		helperMain()
		// helperMain only returns on error; success replaces this process.
		os.Exit(1)
	}
	tmp, err := os.MkdirTemp("", "vsui-launcher-test-*")
	if err != nil {
		panic(err)
	}
	fakeServerBin = filepath.Join(tmp, "fake-server"+exeSuffix)
	build := exec.Command("go", "build", "-o", fakeServerBin, "./tools/fake-server")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		println("build fake server failed:\n" + string(out))
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

func helperMain() {
	cfgPath := os.Getenv("VSUI_LAUNCHER_HELPER_CONFIG")
	instanceID := os.Getenv("VSUI_LAUNCHER_HELPER_INSTANCE")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		return
	}
	if err := Run(context.Background(), cfg, instanceID); err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		return
	}
}

// TestRun_ExecsFakeServer drives the full Run() pipeline (read launch.json,
// build env, chdir, mask, exec) end to end against a real "fake server"
// shell script, in a re-exec'd child process so this test's own process
// image is untouched.
func TestRun_ExecsFakeServer(t *testing.T) {
	dir := t.TempDir()
	const id = "main"
	paths := domain.PathsFor(filepath.Join(dir, "instances"), id)
	for _, d := range []string{paths.Server, paths.Logs, paths.Save} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	outFile := filepath.Join(dir, "out.txt")
	t.Setenv("FAKE_SERVER_DUMP", outFile)

	launch := domain.Launch{
		Version:    domain.LaunchVersion,
		InstanceID: id,
		ServerDir:  paths.Server,
		LogDir:     paths.Logs,
		Args:       []string{"-name", "Our Server", "-password", "s3cret", "-public", "1"},
		BepInEx:    false,
	}
	data, err := json.Marshal(launch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.LaunchFile(), data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfgFile := filepath.Join(dir, "config.yaml")
	cfgYAML := fmt.Sprintf("data_dir: %q\nsupervisor: direct\nfake_server: true\nfake_server_path: %q\n", dir, fakeServerBin)
	if err := os.WriteFile(cfgFile, []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMain$")
	cmd.Env = append(os.Environ(),
		"VSUI_LAUNCHER_HELPER=1",
		"VSUI_LAUNCHER_HELPER_CONFIG="+cfgFile,
		"VSUI_LAUNCHER_HELPER_INSTANCE="+id,
	)
	// The fake server runs for a while; stop it once its dump exists. On
	// Linux the helper *is* the fake server (exec); on Windows the helper is
	// the proxy in front of it and stops it through the stdin protocol.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var outBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, outFile, 20*time.Second)
	stopHelper(t, cmd, stdin)
	out := outBuf.String()
	if !strings.Contains(out, "[valheim-ui] launching") || !strings.Contains(out, "********") {
		t.Errorf("expected masked launch banner on stdout, got: %s", out)
	}

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("fake server did not run: %v", err)
	}
	text := string(got)
	if !strings.Contains(text, "ARGS:-name Our Server -password s3cret -public 1") {
		t.Errorf("unexpected args in fake server output: %s", text)
	}
	if !strings.Contains(text, "STEAMAPPID:892970") {
		t.Errorf("SteamAppId not set: %s", text)
	}
	if runtime.GOOS != "windows" && !strings.Contains(text, "LDLIB:./linux64:") {
		t.Errorf("LD_LIBRARY_PATH not set: %s", text)
	}
	if !strings.Contains(text, "PWD:"+paths.Server) {
		t.Errorf("cwd was not the server dir: %s", text)
	}
}

// waitForFile polls until path exists (the fake server writes its dump as
// its first action).
func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

// TestRun_MissingLaunchFile exercises the plain error path without exec-ing
// anything.
func TestRun_MissingLaunchFile(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.Supervisor = "direct"
	err := run(context.Background(), cfg, "missing", failExec)
	if err == nil {
		t.Fatal("expected an error for a missing launch.json")
	}
}

func TestRun_RequiresInstanceID(t *testing.T) {
	if err := run(context.Background(), config.Default(), "", failExec); err == nil {
		t.Fatal("expected an error for an empty instance id")
	}
}

func failExec(string, []string, []string) error {
	return errors.New("exec should not have been called")
}
