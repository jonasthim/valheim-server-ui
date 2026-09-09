package steam

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func writeFakeSteamCMD(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "steamcmd.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("write fake steamcmd: %v", err)
	}
	return path
}

func TestInstalled(t *testing.T) {
	exe := writeFakeSteamCMD(t, "exit 0")
	c := New(exe, nil)
	if !c.Installed() {
		t.Fatal("expected executable script to be reported installed")
	}

	c2 := New(filepath.Join(t.TempDir(), "does-not-exist.sh"), nil)
	if c2.Installed() {
		t.Fatal("expected missing path to be reported not installed")
	}

	// Not executable.
	dir := t.TempDir()
	notExec := filepath.Join(dir, "steamcmd.sh")
	if err := os.WriteFile(notExec, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c3 := New(notExec, nil)
	if c3.Installed() {
		t.Fatal("expected non-executable file to be reported not installed")
	}
}

func TestInstalledBuildID(t *testing.T) {
	c := New("/bin/true", nil)

	installDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(installDir, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("testdata/appmanifest_896660.acf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dst := filepath.Join(installDir, "steamapps", "appmanifest_896660.acf")
	if err := os.WriteFile(dst, fixture, 0o644); err != nil {
		t.Fatal(err)
	}

	id, err := c.InstalledBuildID(installDir)
	if err != nil {
		t.Fatalf("InstalledBuildID: %v", err)
	}
	if id != "22222222" {
		t.Fatalf("expected buildid 22222222, got %q", id)
	}

	// Missing manifest (not installed) -> "", nil.
	id2, err := c.InstalledBuildID(t.TempDir())
	if err != nil {
		t.Fatalf("InstalledBuildID (missing): %v", err)
	}
	if id2 != "" {
		t.Fatalf("expected empty buildid for missing manifest, got %q", id2)
	}
}

func TestParseLatestBuildID_Fixture(t *testing.T) {
	b, err := os.ReadFile("testdata/app_info.vdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	id, err := parseLatestBuildID(string(b))
	if err != nil {
		t.Fatalf("parseLatestBuildID: %v", err)
	}
	if id != "22222222" {
		t.Fatalf("expected public buildid 22222222 (not the beta or unstable branch), got %q", id)
	}
}

func TestParseLatestBuildID_FallbackRegex(t *testing.T) {
	// No "branches" wrapper at all: findBlock("branches") fails, so this must
	// go through fallbackPublicBuildID.
	snippet := `"public"
	{
		"buildid"		"99999999"
		"timeupdated"		"1700000000"
	}`
	id, err := parseLatestBuildID(snippet)
	if err != nil {
		t.Fatalf("parseLatestBuildID: %v", err)
	}
	if id != "99999999" {
		t.Fatalf("expected fallback to find 99999999, got %q", id)
	}
}

func TestParseLatestBuildID_NotFound(t *testing.T) {
	if _, err := parseLatestBuildID("no vdf content here"); err == nil {
		t.Fatal("expected an error when no buildid can be found")
	}
}

func TestInstallOrUpdate_Success(t *testing.T) {
	exe := writeFakeSteamCMD(t, `echo "Success! App '896660' fully installed"`)
	c := New(exe, nil)
	var out strings.Builder
	if err := c.InstallOrUpdate(context.Background(), t.TempDir(), &out); err != nil {
		t.Fatalf("InstallOrUpdate: %v", err)
	}
	if !strings.Contains(out.String(), "fully installed") {
		t.Fatalf("expected output to be streamed to out, got %q", out.String())
	}
}

func TestInstallOrUpdate_AlreadyUpToDate(t *testing.T) {
	exe := writeFakeSteamCMD(t, `echo "already up to date"`)
	c := New(exe, nil)
	if err := c.InstallOrUpdate(context.Background(), t.TempDir(), io.Discard); err != nil {
		t.Fatalf("InstallOrUpdate: %v", err)
	}
}

func TestInstallOrUpdate_MissingSteamCMD(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "nope.sh"), nil)
	err := c.InstallOrUpdate(context.Background(), t.TempDir(), io.Discard)
	if err == nil {
		t.Fatal("expected an error when steamcmd is missing")
	}
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeSteamCMDMissing {
		t.Fatalf("expected steamcmd_missing error, got %v", err)
	}
}

func TestInstallOrUpdate_FlakeRetriesOnce(t *testing.T) {
	exe := writeFakeSteamCMD(t, "exit 0") // never actually invoked; runner is mocked
	var calls int32
	run := func(ctx context.Context, path string, args []string, out io.Writer) error {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			_, _ = io.WriteString(out, "Error! State is 0x6 after update job 'foo'\n")
			return errors.New("exit status 7")
		}
		_, _ = io.WriteString(out, successMarker+"\n")
		return nil
	}
	c := New(exe, nil, WithCommandRunner(run))
	if err := c.InstallOrUpdate(context.Background(), t.TempDir(), io.Discard); err != nil {
		t.Fatalf("expected the retry to succeed, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", got)
	}
}

func TestInstallOrUpdate_NonFlakeFailureDoesNotRetry(t *testing.T) {
	exe := writeFakeSteamCMD(t, "exit 0")
	var calls int32
	run := func(ctx context.Context, path string, args []string, out io.Writer) error {
		atomic.AddInt32(&calls, 1)
		_, _ = io.WriteString(out, "some unrelated fatal error\n")
		return errors.New("exit status 1")
	}
	c := New(exe, nil, WithCommandRunner(run))
	err := c.InstallOrUpdate(context.Background(), t.TempDir(), io.Discard)
	if err == nil {
		t.Fatal("expected failure")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("non-flake failures must not retry, got %d attempts", got)
	}
	if !strings.Contains(err.Error(), "unrelated fatal error") {
		t.Fatalf("expected error to include last lines of output, got %v", err)
	}
}

func TestInstallOrUpdate_ContextCancelled(t *testing.T) {
	started := make(chan struct{})
	run := func(ctx context.Context, path string, args []string, out io.Writer) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	c := New("/bin/true", nil, WithCommandRunner(run))
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- c.InstallOrUpdate(ctx, t.TempDir(), io.Discard) }()
	<-started
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected error wrapping context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("InstallOrUpdate did not return after ctx cancellation")
	}
}

func TestLatestBuildID_CachesResult(t *testing.T) {
	fixture, err := os.ReadFile("testdata/app_info.vdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	run := func(ctx context.Context, path string, args []string, out io.Writer) error {
		_, _ = out.Write(fixture)
		return nil
	}
	c := New("/bin/true", nil, WithCommandRunner(run))

	if build, at := c.Latest(); build != "" || !at.IsZero() {
		t.Fatalf("expected no cached value before the first check, got %q %v", build, at)
	}

	id, err := c.LatestBuildID(context.Background())
	if err != nil {
		t.Fatalf("LatestBuildID: %v", err)
	}
	if id != "22222222" {
		t.Fatalf("expected 22222222, got %q", id)
	}
	build, at := c.Latest()
	if build != "22222222" {
		t.Fatalf("expected cached buildid 22222222, got %q", build)
	}
	if at.IsZero() || time.Since(at) > time.Minute {
		t.Fatalf("expected a recent checkedAt, got %v", at)
	}
}

func TestLatestBuildID_MissingSteamCMD(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "nope.sh"), nil)
	_, err := c.LatestBuildID(context.Background())
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeSteamCMDMissing {
		t.Fatalf("expected steamcmd_missing error, got %v", err)
	}
}

func TestLastLines(t *testing.T) {
	s := "a\nb\nc\nd\ne\n"
	if got := lastLines(s, 2); got != "d\ne" {
		t.Fatalf("expected last 2 lines, got %q", got)
	}
	if got := lastLines("only one line", 5); got != "only one line" {
		t.Fatalf("expected the whole string when shorter than n, got %q", got)
	}
}
