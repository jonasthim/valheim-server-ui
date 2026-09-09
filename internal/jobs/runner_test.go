package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
)

func testRunner(t *testing.T, opts ...Option) (*Runner, *sql.DB, *events.Bus) {
	t.Helper()
	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	bus := events.NewBus()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	jobsDir := t.TempDir()
	r := New(sqldb, bus, jobsDir, log, opts...)
	return r, sqldb, bus
}

func startRunner(t *testing.T, r *Runner) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = r.Start(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("runner did not shut down after ctx cancel")
		}
	})
	return ctx
}

func waitTerminal(t *testing.T, r *Runner, id string) *domain.Job {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	job, err := r.WaitFor(ctx, id)
	if err != nil {
		t.Fatalf("WaitFor(%s): %v", id, err)
	}
	return job
}

func TestSerialPerInstanceParallelAcrossInstances(t *testing.T) {
	r, _, _ := testRunner(t)
	startRunner(t, r)

	var mu sync.Mutex
	var order []string
	started := make(chan string, 10)
	releaseA1 := make(chan struct{})

	fnA1 := func(ctx context.Context, log *Logger) error {
		mu.Lock()
		order = append(order, "a1-start")
		mu.Unlock()
		started <- "a1"
		<-releaseA1
		mu.Lock()
		order = append(order, "a1-end")
		mu.Unlock()
		return nil
	}
	fnA2 := func(ctx context.Context, log *Logger) error {
		mu.Lock()
		order = append(order, "a2-start")
		mu.Unlock()
		started <- "a2"
		return nil
	}
	fnB1 := func(ctx context.Context, log *Logger) error {
		started <- "b1"
		return nil
	}

	jobA1, err := r.Enqueue(context.Background(), Spec{Type: domain.JobInstall, InstanceID: "a"}, fnA1)
	if err != nil {
		t.Fatalf("enqueue a1: %v", err)
	}
	jobA2, err := r.Enqueue(context.Background(), Spec{Type: domain.JobUpdate, InstanceID: "a"}, fnA2)
	if err != nil {
		t.Fatalf("enqueue a2: %v", err)
	}
	jobB1, err := r.Enqueue(context.Background(), Spec{Type: domain.JobInstall, InstanceID: "b"}, fnB1)
	if err != nil {
		t.Fatalf("enqueue b1: %v", err)
	}

	// a1 starts immediately.
	select {
	case name := <-started:
		if name != "a1" {
			t.Fatalf("expected a1 to start first, got %s", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a1 never started")
	}

	// b1, on a different instance queue, must run concurrently with a1
	// (which is still blocked), proving queues are independent.
	select {
	case name := <-started:
		if name != "b1" {
			t.Fatalf("expected b1 to start while a1 is blocked, got %s", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("b1 never started even though it is on a different instance queue")
	}

	// a2 must NOT start yet: it is serialised behind a1 on the same queue.
	select {
	case name := <-started:
		t.Fatalf("a2 must wait for a1 to finish, but %s started", name)
	case <-time.After(150 * time.Millisecond):
	}

	close(releaseA1)

	select {
	case name := <-started:
		if name != "a2" {
			t.Fatalf("expected a2 to start after a1 finished, got %s", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a2 never started after a1 finished")
	}

	waitTerminal(t, r, jobA1.ID)
	waitTerminal(t, r, jobA2.ID)
	waitTerminal(t, r, jobB1.ID)

	mu.Lock()
	defer mu.Unlock()
	// a1 must fully finish (a1-end) before a2 starts.
	idxEnd, idxStart := -1, -1
	for i, s := range order {
		if s == "a1-end" {
			idxEnd = i
		}
		if s == "a2-start" {
			idxStart = i
		}
	}
	if idxEnd == -1 || idxStart == -1 || idxEnd > idxStart {
		t.Fatalf("expected a1-end before a2-start, got order %v", order)
	}
}

func TestMaxConcurrencyCap(t *testing.T) {
	r, _, _ := testRunner(t, WithMaxConcurrent(2))
	startRunner(t, r)

	var running int32
	var maxSeen int32
	release := make(chan struct{})
	block := func(ctx context.Context, log *Logger) error {
		n := atomic.AddInt32(&running, 1)
		for {
			old := atomic.LoadInt32(&maxSeen)
			if n <= old || atomic.CompareAndSwapInt32(&maxSeen, old, n) {
				break
			}
		}
		<-release
		atomic.AddInt32(&running, -1)
		return nil
	}

	var ids []string
	for i, inst := range []string{"i1", "i2", "i3"} {
		job, err := r.Enqueue(context.Background(), Spec{Type: domain.JobInstall, InstanceID: inst}, block)
		if err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
		ids = append(ids, job.ID)
	}

	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&running) != 2 {
		select {
		case <-deadline:
			t.Fatalf("never reached 2 concurrently running jobs (running=%d)", atomic.LoadInt32(&running))
		case <-time.After(10 * time.Millisecond):
		}
	}
	// Give the (would-be) third worker a chance to wrongly start.
	time.Sleep(150 * time.Millisecond)
	if got := atomic.LoadInt32(&running); got != 2 {
		t.Fatalf("expected exactly 2 running with cap=2, got %d", got)
	}

	close(release)
	for _, id := range ids {
		waitTerminal(t, r, id)
	}
	if atomic.LoadInt32(&maxSeen) > 2 {
		t.Fatalf("observed more than 2 concurrent jobs: %d", maxSeen)
	}
}

func TestCancelQueuedJobNeverRuns(t *testing.T) {
	r, _, _ := testRunner(t)
	startRunner(t, r)

	blocker := make(chan struct{})
	blockingFn := func(ctx context.Context, log *Logger) error {
		<-blocker
		return nil
	}
	first, err := r.Enqueue(context.Background(), Spec{Type: domain.JobInstall, InstanceID: "x"}, blockingFn)
	if err != nil {
		t.Fatalf("enqueue first: %v", err)
	}

	var ran atomic.Bool
	second, err := r.Enqueue(context.Background(), Spec{Type: domain.JobUpdate, InstanceID: "x"}, func(ctx context.Context, log *Logger) error {
		ran.Store(true)
		return nil
	})
	if err != nil {
		t.Fatalf("enqueue second: %v", err)
	}

	cancelled, err := r.Cancel(context.Background(), second.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Status != domain.JobCancelled {
		t.Fatalf("expected cancelled status immediately, got %s", cancelled.Status)
	}

	close(blocker)
	waitTerminal(t, r, first.ID)

	final := waitTerminal(t, r, second.ID)
	if final.Status != domain.JobCancelled {
		t.Fatalf("expected final status cancelled, got %s", final.Status)
	}
	if ran.Load() {
		t.Fatal("cancelled-while-queued job's Func must never run")
	}
	if lines, err := r.Log(context.Background(), second.ID); err != nil || len(lines) != 0 {
		t.Fatalf("expected no log lines for a job that never ran, got %v err=%v", lines, err)
	}
}

func TestCancelRunningJob_ErrorPropagatesAsCancelled(t *testing.T) {
	r, _, _ := testRunner(t)
	startRunner(t, r)

	started := make(chan struct{})
	fn := func(ctx context.Context, log *Logger) error {
		close(started)
		<-ctx.Done()
		return fmt.Errorf("aborting: %w", ctx.Err())
	}
	job, err := r.Enqueue(context.Background(), Spec{Type: domain.JobInstall, InstanceID: "y"}, fn)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	<-started
	if _, err := r.Cancel(context.Background(), job.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	final := waitTerminal(t, r, job.ID)
	if final.Status != domain.JobCancelled {
		t.Fatalf("expected cancelled, got %s (error=%q)", final.Status, final.Error)
	}
}

func TestCancelRunningJob_NilReturnStillCancelled(t *testing.T) {
	r, _, _ := testRunner(t)
	startRunner(t, r)

	started := make(chan struct{})
	fn := func(ctx context.Context, log *Logger) error {
		close(started)
		<-ctx.Done()
		return nil // ignores the cancellation in its return value
	}
	job, err := r.Enqueue(context.Background(), Spec{Type: domain.JobInstall, InstanceID: "z"}, fn)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	<-started
	if _, err := r.Cancel(context.Background(), job.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	final := waitTerminal(t, r, job.ID)
	if final.Status != domain.JobCancelled {
		t.Fatalf("expected cancelled even though Func returned nil, got %s", final.Status)
	}
}

func TestLogFileAndEvents(t *testing.T) {
	r, _, bus := testRunner(t)
	startRunner(t, r)

	sub := bus.Subscribe("")
	defer sub.Close()

	fn := func(ctx context.Context, log *Logger) error {
		log.Printf("hello %d", 1)
		w := log.Writer()
		_, _ = w.Write([]byte("line one\nline "))
		_, _ = w.Write([]byte("two\n"))
		log.SetSummary("k", "v")
		return nil
	}
	job, err := r.Enqueue(context.Background(), Spec{Type: domain.JobBackup, InstanceID: "log1"}, fn)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	final := waitTerminal(t, r, job.ID)
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected succeeded, got %s (%s)", final.Status, final.Error)
	}
	if final.Summary["k"] != "v" {
		t.Fatalf("expected summary to be persisted, got %v", final.Summary)
	}

	lines, err := r.Log(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"hello 1", "line one", "line two"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected log to contain %q, got:\n%s", want, joined)
		}
	}

	var gotLines []string
	var gotUpdated int
	deadline := time.After(2 * time.Second)
collect:
	for {
		select {
		case ev := <-sub.C:
			switch ev.Name {
			case domain.EventJobLog:
				payload, ok := ev.Data.(jobLogPayload)
				if !ok {
					t.Fatalf("unexpected job.log payload type %T", ev.Data)
				}
				if payload.JobID != job.ID {
					t.Fatalf("job.log for wrong job: %s", payload.JobID)
				}
				gotLines = append(gotLines, payload.Line)
			case domain.EventJobUpdated:
				gotUpdated++
				if final, ok := ev.Data.(domain.Job); ok && final.ID == job.ID && final.Status.Terminal() {
					break collect
				}
			}
		case <-deadline:
			t.Fatal("timed out waiting for job.log/job.updated events")
		}
	}
	joinedEv := strings.Join(gotLines, "\n")
	for _, want := range []string{"hello 1", "line one", "line two"} {
		if !strings.Contains(joinedEv, want) {
			t.Fatalf("expected a job.log event containing %q, got:\n%s", want, joinedEv)
		}
	}
	if gotUpdated < 2 { // queued->running (from Enqueue+markRunning) .. succeeded
		t.Fatalf("expected at least 2 job.updated events, got %d", gotUpdated)
	}
}

// TestStartDoesNotFailJobsEnqueuedBeforeRecovery pins the startup race that
// made TestLogFileAndEvents flaky: a job enqueued by this process must never
// be reported as a stale "manager restarted" failure by Start's recovery.
func TestStartDoesNotFailJobsEnqueuedBeforeRecovery(t *testing.T) {
	r, sqldb, bus := testRunner(t)
	sub := bus.Subscribe("")
	defer sub.Close()

	release := make(chan struct{})
	fn := func(ctx context.Context, log *Logger) error {
		select {
		case <-release:
		case <-ctx.Done():
		}
		log.Printf("done")
		return nil
	}
	// Enqueued before Start: the row is queued/running in the DB exactly when
	// recovery looks for leftovers from a previous process.
	job, err := r.Enqueue(context.Background(), Spec{Type: domain.JobBackup, InstanceID: "pre"}, fn)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	startRunner(t, r)

	// Give recovery a real chance to run before the job finishes.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		var status string
		if err := sqldb.QueryRow(`SELECT status FROM jobs WHERE id = ?`, job.ID).Scan(&status); err != nil {
			t.Fatalf("query status: %v", err)
		}
		if status == string(domain.JobFailed) {
			t.Fatalf("recovery marked a job owned by this process as failed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)

	final := waitTerminal(t, r, job.ID)
	if final.Status != domain.JobSucceeded {
		t.Fatalf("expected succeeded, got %s (%s)", final.Status, final.Error)
	}
	// No job.updated event for this job may carry the stale-recovery failure.
	for {
		select {
		case ev := <-sub.C:
			if ev.Name != domain.EventJobUpdated {
				continue
			}
			if j, ok := ev.Data.(domain.Job); ok && j.ID == job.ID {
				if j.Status == domain.JobFailed {
					t.Fatalf("got a spurious failed event: %q", j.Error)
				}
				if j.Status.Terminal() {
					return
				}
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for the terminal job.updated event")
		}
	}
}

func TestRecoveryOnStart(t *testing.T) {
	sqldb, err := db.OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	defer sqldb.Close()

	now := time.Now().UTC().Format(timeLayout)
	if _, err := sqldb.Exec(`INSERT INTO jobs (id, type, instance_id, status, title, requested_by, created_at, started_at, finished_at, error, summary_json)
		VALUES ('stale-running', 'install', 'inst1', 'running', 'Install', 'alice', ?, ?, NULL, '', '{}')`, now, now); err != nil {
		t.Fatalf("seed running job: %v", err)
	}
	if _, err := sqldb.Exec(`INSERT INTO jobs (id, type, instance_id, status, title, requested_by, created_at, started_at, finished_at, error, summary_json)
		VALUES ('stale-queued', 'update', 'inst1', 'queued', 'Update', 'bob', ?, NULL, NULL, '', '{}')`, now); err != nil {
		t.Fatalf("seed queued job: %v", err)
	}
	if _, err := sqldb.Exec(`INSERT INTO jobs (id, type, instance_id, status, title, requested_by, created_at, started_at, finished_at, error, summary_json)
		VALUES ('already-done', 'backup', 'inst1', 'succeeded', 'Backup', 'carol', ?, ?, ?, '', '{}')`, now, now, now); err != nil {
		t.Fatalf("seed succeeded job: %v", err)
	}

	bus := events.NewBus()
	sub := bus.Subscribe("")
	defer sub.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	r := New(sqldb, bus, t.TempDir(), log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { _ = r.Start(ctx); close(done) }()

	seenFailed := map[string]bool{}
	deadline := time.After(2 * time.Second)
	for len(seenFailed) < 2 {
		select {
		case ev := <-sub.C:
			if ev.Name != domain.EventJobUpdated {
				continue
			}
			j, ok := ev.Data.(domain.Job)
			if !ok {
				continue
			}
			if j.Status == domain.JobFailed && j.Error == "manager restarted" {
				seenFailed[j.ID] = true
			}
		case <-deadline:
			t.Fatalf("timed out waiting for recovery events, got %v", seenFailed)
		}
	}
	if !seenFailed["stale-running"] || !seenFailed["stale-queued"] {
		t.Fatalf("expected both stale jobs recovered, got %v", seenFailed)
	}

	for _, id := range []string{"stale-running", "stale-queued"} {
		job, err := r.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if job.Status != domain.JobFailed || job.Error != "manager restarted" {
			t.Fatalf("job %s not recovered correctly: %+v", id, job)
		}
		if job.FinishedAt == nil {
			t.Fatalf("job %s missing finished_at", id)
		}
	}

	untouched, err := r.Get(context.Background(), "already-done")
	if err != nil {
		t.Fatalf("get already-done: %v", err)
	}
	if untouched.Status != domain.JobSucceeded {
		t.Fatalf("recovery must not touch already-terminal jobs, got %s", untouched.Status)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runner did not shut down")
	}
}

func TestListFilters(t *testing.T) {
	r, _, _ := testRunner(t)
	startRunner(t, r)

	noop := func(ctx context.Context, log *Logger) error { return nil }
	blocker := make(chan struct{})
	block := func(ctx context.Context, log *Logger) error { <-blocker; return nil }

	var ids []string
	mustEnqueue := func(spec Spec, fn Func) *domain.Job {
		job, err := r.Enqueue(context.Background(), spec, fn)
		if err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		ids = append(ids, job.ID)
		return job
	}

	jA1 := mustEnqueue(Spec{Type: domain.JobInstall, InstanceID: "alpha"}, noop)
	jA2 := mustEnqueue(Spec{Type: domain.JobUpdate, InstanceID: "alpha"}, block)
	jB1 := mustEnqueue(Spec{Type: domain.JobInstall, InstanceID: "beta"}, noop)
	jG1 := mustEnqueue(Spec{Type: domain.JobThunderstoreRefresh, InstanceID: ""}, noop)

	waitTerminal(t, r, jA1.ID)
	waitTerminal(t, r, jB1.ID)
	waitTerminal(t, r, jG1.ID)
	// jA2 is queued behind nothing but its Func blocks, so it's running now;
	// leave it running to exercise the status filter, then release it.
	deadline := time.After(2 * time.Second)
	for {
		j, err := r.Get(context.Background(), jA2.ID)
		if err != nil {
			t.Fatalf("get jA2: %v", err)
		}
		if j.Status == domain.JobRunning {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("jA2 never reached running, got %s", j.Status)
		case <-time.After(10 * time.Millisecond):
		}
	}

	all, err := r.List(context.Background(), "", "", 0)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 jobs total, got %d", len(all))
	}
	sortedByTime := sort.SliceIsSorted(all, func(i, j int) bool {
		return !all[i].CreatedAt.Before(all[j].CreatedAt) // descending (newest first)
	})
	if !sortedByTime {
		t.Fatalf("expected newest-first order, got %+v", all)
	}

	alphaOnly, err := r.List(context.Background(), "alpha", "", 0)
	if err != nil {
		t.Fatalf("list alpha: %v", err)
	}
	if len(alphaOnly) != 2 {
		t.Fatalf("expected 2 alpha jobs, got %d", len(alphaOnly))
	}
	for _, j := range alphaOnly {
		if j.InstanceID != "alpha" {
			t.Fatalf("list instance filter leaked job %+v", j)
		}
	}

	running, err := r.List(context.Background(), "", domain.JobRunning, 0)
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(running) != 1 || running[0].ID != jA2.ID {
		t.Fatalf("expected exactly jA2 running, got %+v", running)
	}

	limited, err := r.List(context.Background(), "", "", 2)
	if err != nil {
		t.Fatalf("list limited: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("expected limit=2 to return 2 jobs, got %d", len(limited))
	}

	close(blocker)
	waitTerminal(t, r, jA2.ID)
}

func TestEnqueueExclusiveRejectsWhenActive(t *testing.T) {
	r, _, _ := testRunner(t)
	startRunner(t, r)

	blocker := make(chan struct{})
	first, err := r.Enqueue(context.Background(), Spec{Type: domain.JobInstall, InstanceID: "excl"}, func(ctx context.Context, log *Logger) error {
		<-blocker
		return nil
	})
	if err != nil {
		t.Fatalf("enqueue first: %v", err)
	}

	_, err = r.Enqueue(context.Background(), Spec{Type: domain.JobUpdate, InstanceID: "excl", Exclusive: true}, func(ctx context.Context, log *Logger) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected exclusive enqueue to be rejected while a job is active")
	}
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeInstanceBusy {
		t.Fatalf("expected instance_busy error, got %v", err)
	}

	close(blocker)
	waitTerminal(t, r, first.ID)
}

func TestJobLogUnknownID(t *testing.T) {
	r, _, _ := testRunner(t)
	if _, err := r.Log(context.Background(), "does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown job id")
	}
}

func TestJobsDirCreatedWithLogFile(t *testing.T) {
	r, _, _ := testRunner(t)
	startRunner(t, r)

	job, err := r.Enqueue(context.Background(), Spec{Type: domain.JobInstall, InstanceID: "dirtest"}, func(ctx context.Context, log *Logger) error {
		log.Printf("hi")
		return nil
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	waitTerminal(t, r, job.ID)
	path := filepath.Join(r.jobsDir, job.ID+".log")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected log file to exist at %s: %v", path, err)
	}
}
