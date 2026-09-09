package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

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
	os.Exit(m.Run())
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
	fakeScript := filepath.Join(dir, "fake_server.sh")
	script := "#!/bin/sh\n" +
		"{\n" +
		"  echo \"ARGS:$*\"\n" +
		"  echo \"STEAMAPPID:$SteamAppId\"\n" +
		"  echo \"LDLIB:$LD_LIBRARY_PATH\"\n" +
		"  echo \"PWD:$(pwd)\"\n" +
		"} > \"" + outFile + "\"\n"
	if err := os.WriteFile(fakeScript, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

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
	cfgYAML := fmt.Sprintf("data_dir: %q\nsupervisor: direct\nfake_server: true\nfake_server_path: %q\n", dir, fakeScript)
	if err := os.WriteFile(cfgFile, []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMain$")
	cmd.Env = append(os.Environ(),
		"VSUI_LAUNCHER_HELPER=1",
		"VSUI_LAUNCHER_HELPER_CONFIG="+cfgFile,
		"VSUI_LAUNCHER_HELPER_INSTANCE="+id,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper process failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(string(out), "[valheim-ui] launching") || !strings.Contains(string(out), "********") {
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
	if !strings.Contains(text, "LDLIB:./linux64:") {
		t.Errorf("LD_LIBRARY_PATH not set: %s", text)
	}
	if !strings.Contains(text, "PWD:"+paths.Server) {
		t.Errorf("cwd was not the server dir: %s", text)
	}
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
