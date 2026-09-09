package selfupdate

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// serviceHarness wires a Service against a real jobs.Runner (in-memory DB)
// and a fake GitHub server, with an exit hook that records calls instead of
// exiting the test process. assetDelay slows down the fake download so a
// test can observe the job while it is still running.
func serviceHarness(t *testing.T, tag, currentVersion string, assetDelay time.Duration) (*Service, *jobs.Runner, *int32, func()) {
	t.Helper()
	dir := t.TempDir()
	exe := installedBinary(t, dir, currentVersion)

	tarball, sums := buildReleaseAssets(t, tag)
	srv := newGitHubServer(t, githubServerOptions{tag: tag, tarball: tarball, sums: sums, assetDelay: assetDelay})
	client := NewClient(domain.GitHubRepo, currentVersion, WithBaseURL(srv.URL))

	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	bus := events.NewBus()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	runner := jobs.New(sqldb, bus, t.TempDir(), log)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = runner.Start(ctx)
		close(done)
	}()
	stop := func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("runner did not shut down")
		}
	}

	checker := New(client, currentVersion, exe,
		func() time.Duration { return 0 }, func() bool { return false },
		func(context.Context) (int, error) { return 0, nil }, bus, nil)
	upgrader := NewUpgrader(exe, currentVersion, http.DefaultClient)

	var exitCalls int32
	svc := NewService(client, checker, upgrader, runner, nil, WithExit(func(int) {
		atomic.AddInt32(&exitCalls, 1)
	}))
	return svc, runner, &exitCalls, stop
}

func TestService_EnqueueUpgrade_HappyPath(t *testing.T) {
	svc, runner, exitCalls, stop := serviceHarness(t, "v2.0.0", "v1.0.0", 0)
	defer stop()

	job, err := svc.EnqueueUpgrade(context.Background(), "", "alice")
	if err != nil {
		t.Fatalf("EnqueueUpgrade: %v", err)
	}
	if job.Type != domain.JobSelfUpgrade {
		t.Fatalf("expected job type self_upgrade, got %s", job.Type)
	}

	final, err := runner.WaitFor(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected job to succeed, got %s (%s)", final.Status, final.Error)
	}
	if final.Summary["from"] != "v1.0.0" || final.Summary["to"] != "v2.0.0" {
		t.Fatalf("unexpected summary: %+v", final.Summary)
	}

	// The exit hook is deferred by 2s; it must not have fired yet, but it
	// also must not be forgotten. Poll briefly instead of sleeping the full
	// 2s in the common case.
	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt32(exitCalls) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := atomic.LoadInt32(exitCalls); got != 1 {
		t.Fatalf("expected exit to be called exactly once after the job finished, got %d", got)
	}
}

func TestService_EnqueueUpgrade_RefusesDoubleEnqueue(t *testing.T) {
	// The fake download is slowed down so the first job is still running
	// (not yet succeeded) when the second EnqueueUpgrade call checks
	// ActiveFor, making the refusal deterministic instead of a timing race.
	svc, runner, _, stop := serviceHarness(t, "v2.0.0", "v1.0.0", 500*time.Millisecond)
	defer stop()

	job, err := svc.EnqueueUpgrade(context.Background(), "", "alice")
	if err != nil {
		t.Fatalf("first EnqueueUpgrade: %v", err)
	}
	_, err = svc.EnqueueUpgrade(context.Background(), "", "bob")
	if err == nil {
		t.Fatal("expected the second concurrent EnqueueUpgrade to be refused")
	}
	de := domain.AsError(err)
	if de.Code != domain.CodeConflict {
		t.Fatalf("expected conflict, got %s", de.Code)
	}

	// Let the first job finish so stop() doesn't race its own shutdown.
	if _, err := runner.WaitFor(context.Background(), job.ID); err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
}

func TestService_EnqueueUpgrade_NoReleases(t *testing.T) {
	dir := t.TempDir()
	exe := installedBinary(t, dir, "v1.0.0")
	srv := newNotFoundServer(t)
	client := NewClient(domain.GitHubRepo, "v1.0.0", WithBaseURL(srv.URL))

	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	runner := jobs.New(sqldb, nil, t.TempDir(), nil)

	checker := New(client, "v1.0.0", exe, func() time.Duration { return 0 }, func() bool { return false }, nil, nil, nil)
	upgrader := NewUpgrader(exe, "v1.0.0", http.DefaultClient)
	svc := NewService(client, checker, upgrader, runner, nil)

	_, err = svc.EnqueueUpgrade(context.Background(), "", "alice")
	if err == nil {
		t.Fatal("expected an error when there are no releases yet")
	}
	if domain.AsError(err).Code != domain.CodeConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestService_EnqueueUpgrade_RefusesWhenCannotSelfUpgrade(t *testing.T) {
	dir := t.TempDir()
	exe := installedBinary(t, dir, "dev") // "dev" -> CanSelfUpgrade == false

	tarball, sums := buildReleaseAssets(t, "v2.0.0")
	srv := newGitHubServer(t, githubServerOptions{tag: "v2.0.0", tarball: tarball, sums: sums})
	client := NewClient(domain.GitHubRepo, "dev", WithBaseURL(srv.URL))

	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	runner := jobs.New(sqldb, nil, t.TempDir(), nil)

	checker := New(client, "dev", exe, func() time.Duration { return 0 }, func() bool { return false }, nil, nil, nil)
	upgrader := NewUpgrader(exe, "dev", http.DefaultClient)
	svc := NewService(client, checker, upgrader, runner, nil)

	_, err = svc.EnqueueUpgrade(context.Background(), "v2.0.0", "alice")
	if err == nil {
		t.Fatal("expected a dev build to refuse self-upgrade")
	}
	de := domain.AsError(err)
	if de.Code != domain.CodeConflict {
		t.Fatalf("expected conflict, got %s", de.Code)
	}
}
