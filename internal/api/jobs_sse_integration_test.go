package api_test

// Manual-check-as-a-test (see docs/WORKPLAN.md WP-04 "Done when"): enqueues a
// job that runs a fake steamcmd-like script (prints a few lines, sleeps
// between them) through the real jobs.Runner, and observes its job.log /
// job.updated events arrive framed as `curl -N /api/v1/events` would see
// them, via the real SSE handler wired through api.NewRouter.

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// syncRecorder is a minimal, mutex-guarded http.ResponseWriter+http.Flusher
// so a test goroutine can read the body while the SSE handler goroutine is
// still writing (see internal/api's own safeRecorder for the same need).
type syncRecorder struct {
	mu     sync.Mutex
	header http.Header
	sb     strings.Builder
}

func newSyncRecorder() *syncRecorder        { return &syncRecorder{header: http.Header{}} }
func (s *syncRecorder) Header() http.Header { return s.header }
func (s *syncRecorder) WriteHeader(int)     {}
func (s *syncRecorder) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sb.Write(p)
}
func (s *syncRecorder) Flush() {}
func (s *syncRecorder) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sb.String()
}

func TestJobLogStreamsOverSSE_FakeSteamCMDScript(t *testing.T) {
	ctx := context.Background()
	sqldb, err := db.OpenMemory(ctx)
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	defer sqldb.Close()

	bus := events.NewBus()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	runner := jobs.New(sqldb, bus, t.TempDir(), log)

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	runnerDone := make(chan struct{})
	go func() { _ = runner.Start(runCtx); close(runnerDone) }()

	deps := &api.Deps{Cfg: config.Config{}, Log: log, DB: sqldb, Bus: bus, Jobs: runner}
	router := api.NewRouter(deps, nil)

	// Subscribe over SSE first, exactly like `curl -N /api/v1/events`.
	sseCtx, cancelSSE := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(sseCtx)
	rec := newSyncRecorder()
	sseDone := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(sseDone)
	}()
	deadline := time.After(2 * time.Second)
	for bus.SubscriberCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("SSE handler never subscribed")
		case <-time.After(5 * time.Millisecond):
		}
	}

	// A fake steamcmd: prints a few lines with small sleeps in between, the
	// same shape a real `+app_update ... validate` run streams.
	scriptPath := filepath.Join(t.TempDir(), "fake-steamcmd.sh")
	script := "#!/bin/sh\necho 'Redirecting stderr to steamcmd log'\nsleep 0.02\necho 'Update state (0x5) validating...'\nsleep 0.02\necho \"Success! App '896660' fully installed.\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake steamcmd script: %v", err)
	}

	job, err := runner.Enqueue(context.Background(), jobs.Spec{Type: domain.JobInstall, InstanceID: "inst-sse"}, func(ctx context.Context, l *jobs.Logger) error {
		cmd := exec.CommandContext(ctx, scriptPath)
		cmd.Stdout = l.Writer()
		cmd.Stderr = l.Writer()
		return cmd.Run()
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	waitCtx, cancelWait := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelWait()
	final, err := runner.WaitFor(waitCtx, job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected the fake steamcmd job to succeed, got %s (%s)", final.Status, final.Error)
	}

	for _, want := range []string{
		`event: job.log`,
		`Redirecting stderr to steamcmd log`,
		`Update state (0x5) validating`,
		`Success! App '896660' fully installed`,
		`event: job.updated`,
	} {
		deadline := time.After(2 * time.Second)
		for !strings.Contains(rec.String(), want) {
			select {
			case <-deadline:
				t.Fatalf("timed out waiting for SSE stream to contain %q; got:\n%s", want, rec.String())
			case <-time.After(5 * time.Millisecond):
			}
		}
	}

	cancelSSE()
	select {
	case <-sseDone:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not exit after client disconnect")
	}

	cancelRun()
	select {
	case <-runnerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("jobs runner did not shut down")
	}
}
