//go:build windows

package launcher

import (
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func writeFile(path, body string) error { return os.WriteFile(path, []byte(body), 0o600) }

// prepareHelper mirrors supervisor.prepareCommand: a hidden console of its
// own for the proxy, so its console Ctrl+C stays with the fake server.
func prepareHelper(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

const exeSuffix = ".exe"

// stopHelper speaks the proxy's stdin protocol: "stop" becomes a console
// Ctrl+C for the fake server, and the proxy exits with its exit code.
func stopHelper(t *testing.T, cmd *exec.Cmd, stdin io.WriteCloser) {
	t.Helper()
	time.Sleep(200 * time.Millisecond)
	if _, err := io.WriteString(stdin, "stop\n"); err != nil {
		t.Fatalf("write stop: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper process failed: %v", err)
	}
}

func TestPlatformDoorstop(t *testing.T) {
	dir := t.TempDir()
	base := []string{"-name", "x"}

	env, args := platformDoorstop(domainLaunch(dir, true, base), nil)
	want := []string{"-name", "x", "--doorstop-enabled", "true", "--doorstop-target-assembly", dir + `\BepInEx\core\BepInEx.Preloader.dll`}
	if !equal(args, want) {
		t.Fatalf("enabled: got %q want %q", args, want)
	}
	if len(env) != 0 {
		t.Fatalf("enabled: env should be untouched, got %q", env)
	}

	// Mods off, no pack installed: nothing added.
	_, args = platformDoorstop(domainLaunch(dir, false, base), nil)
	if !equal(args, base) {
		t.Fatalf("no pack: got %q want %q", args, base)
	}

	// Mods off but the pack's winhttp.dll proxy is present: disable it.
	if err := writeFile(dir+`\winhttp.dll`, "x"); err != nil {
		t.Fatal(err)
	}
	env, args = platformDoorstop(domainLaunch(dir, false, base), []string{"A=1"})
	if !equal(args, []string{"-name", "x", "--doorstop-enabled", "false"}) {
		t.Fatalf("disabled: got %q", args)
	}
	if !equal(env, []string{"A=1", "DOORSTOP_ENABLED=0"}) {
		t.Fatalf("disabled: env got %q", env)
	}
	if !equal(base, []string{"-name", "x"}) {
		t.Fatalf("launch args were mutated: %q", base)
	}
}
