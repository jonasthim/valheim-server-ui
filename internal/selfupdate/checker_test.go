package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// fakeBus records published events.
type fakeBus struct {
	events []domain.Event
}

func (b *fakeBus) Publish(ev domain.Event) { b.events = append(b.events, ev) }

func TestCanSelfUpgrade_DevelopmentBuild(t *testing.T) {
	dir := t.TempDir()
	exe := installedBinary(t, dir, "dev")
	ok, reason := canSelfUpgrade(exe, "dev")
	if ok {
		t.Fatal("expected dev build to refuse self-upgrade")
	}
	if reason != "development build" {
		t.Fatalf("expected 'development build', got %q", reason)
	}
}

func TestCanSelfUpgrade_MissingBinary(t *testing.T) {
	dir := t.TempDir()
	ok, reason := canSelfUpgrade(filepath.Join(dir, "does-not-exist"), "v1.0.0")
	if ok {
		t.Fatal("expected a missing binary to refuse self-upgrade")
	}
	if reason != "release asset missing" {
		t.Fatalf("expected 'release asset missing', got %q", reason)
	}
}

func TestCanSelfUpgrade_ReadOnlyDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits; this case cannot be exercised as root")
	}
	dir := t.TempDir()
	exe := installedBinary(t, dir, "v1.0.0")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) // let TempDir clean up

	ok, reason := canSelfUpgrade(exe, "v1.0.0")
	if ok {
		t.Fatal("expected a read-only directory to refuse self-upgrade")
	}
	wantPrefix := "binary directory not writable: "
	if len(reason) < len(wantPrefix) || reason[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("expected reason to start with %q, got %q", wantPrefix, reason)
	}
}

// TestDirWritable_NonexistentDir exercises the "not writable" branch of
// dirWritable/canSelfUpgrade in a way that holds regardless of privilege
// (unlike a chmod 0555 directory, root cannot write into a directory that
// simply does not exist either), so it still gives coverage of that branch
// when TestCanSelfUpgrade_ReadOnlyDir above skips itself as root.
func TestDirWritable_NonexistentDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if dirWritable(missing) {
		t.Fatalf("expected dirWritable(%s) to be false for a nonexistent directory", missing)
	}
}

func TestCanSelfUpgrade_WritableDirRealVersion(t *testing.T) {
	dir := t.TempDir()
	exe := installedBinary(t, dir, "v1.0.0")
	ok, reason := canSelfUpgrade(exe, "v1.0.0")
	if !ok {
		t.Fatalf("expected self-upgrade to be possible, got reason %q", reason)
	}
	if reason != "" {
		t.Fatalf("expected no reason when self-upgrade is possible, got %q", reason)
	}
}

// checkerHarness wires a Checker against a fake GitHub server for the
// publish/auto-upgrade behaviour tests below.
func checkerHarness(t *testing.T, tag, currentVersion string, autoUpgrade bool, playersOnline int) (*Checker, *fakeBus, *int32) {
	t.Helper()
	dir := t.TempDir()
	exe := installedBinary(t, dir, currentVersion)

	tarball, sums := buildReleaseAssets(t, tag)
	srv := newGitHubServer(t, githubServerOptions{tag: tag, tarball: tarball, sums: sums})
	client := NewClient(domain.GitHubRepo, currentVersion, WithBaseURL(srv.URL))

	bus := &fakeBus{}
	var upgradeCalls int32
	c := New(client, currentVersion, exe,
		func() time.Duration { return 0 },
		func() bool { return autoUpgrade },
		func(context.Context) (int, error) { return playersOnline, nil },
		bus, nil,
	)
	c.SetUpgradeFunc(func(context.Context, string) error {
		atomic.AddInt32(&upgradeCalls, 1)
		return nil
	})
	return c, bus, &upgradeCalls
}

func TestChecker_PublishesOncePerVersion(t *testing.T) {
	c, bus, _ := checkerHarness(t, "v2.0.0", "v1.0.0", false, 0)

	for i := 0; i < 3; i++ {
		if _, err := c.CheckNow(context.Background()); err != nil {
			t.Fatalf("CheckNow: %v", err)
		}
	}

	count := 0
	for _, ev := range bus.events {
		if ev.Name == domain.EventAppUpdateAvailable {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 app.update_available event across 3 checks, got %d", count)
	}
}

func TestChecker_AutoUpgradeOnlyWhenEnabledAndEmpty(t *testing.T) {
	// Disabled: never calls the upgrade hook even though a newer version exists.
	c, _, calls := checkerHarness(t, "v2.0.0", "v1.0.0", false, 0)
	if _, err := c.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("expected no auto-upgrade when disabled, got %d calls", got)
	}

	// Enabled but players online: still refuses.
	c, _, calls = checkerHarness(t, "v2.0.0", "v1.0.0", true, 2)
	if _, err := c.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("expected no auto-upgrade with players online, got %d calls", got)
	}

	// Enabled and empty: upgrades.
	c, _, calls = checkerHarness(t, "v2.0.0", "v1.0.0", true, 0)
	if _, err := c.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("expected exactly 1 auto-upgrade call, got %d", got)
	}
}

func TestChecker_NeverAutoUpgradesFromDev(t *testing.T) {
	c, _, calls := checkerHarness(t, "v2.0.0", "dev", true, 0)
	info, err := c.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if info.CanSelfUpgrade {
		t.Fatal("expected CanSelfUpgrade to be false for a dev build")
	}
	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("expected no auto-upgrade from a dev build, got %d calls", got)
	}
}

func TestChecker_InfoNeverNil(t *testing.T) {
	dir := t.TempDir()
	exe := installedBinary(t, dir, "v1.0.0")
	c := New(NewClient(domain.GitHubRepo, "v1.0.0"), "v1.0.0", exe,
		func() time.Duration { return 0 }, func() bool { return false },
		nil, nil, nil)
	info := c.Info()
	if info == nil {
		t.Fatal("expected Info() to never return nil")
	}
	if info.CurrentVersion != "v1.0.0" {
		t.Fatalf("expected CurrentVersion v1.0.0, got %q", info.CurrentVersion)
	}
	if !info.CanSelfUpgrade {
		t.Fatalf("expected CanSelfUpgrade true, got reason %q", info.Reason)
	}
}

func TestChecker_PlayersOnlineErrorDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	exe := installedBinary(t, dir, "v1.0.0")
	tarball, sums := buildReleaseAssets(t, "v2.0.0")
	srv := newGitHubServer(t, githubServerOptions{tag: "v2.0.0", tarball: tarball, sums: sums})
	client := NewClient(domain.GitHubRepo, "v1.0.0", WithBaseURL(srv.URL))

	c := New(client, "v1.0.0", exe,
		func() time.Duration { return 0 },
		func() bool { return true },
		func(context.Context) (int, error) { return 0, errors.New("boom") },
		nil, nil,
	)
	upgraded := false
	c.SetUpgradeFunc(func(context.Context, string) error {
		upgraded = true
		return nil
	})
	if _, err := c.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow should not fail when playersOnline errors: %v", err)
	}
	if upgraded {
		t.Fatal("expected no auto-upgrade when playersOnline errors")
	}
}
