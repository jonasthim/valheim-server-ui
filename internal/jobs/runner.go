// Package jobs implements the job runner described in ARCHITECTURE.md §9: one
// serial queue per instance (global jobs use instance id ""), at most
// maxConcurrent jobs running at once overall, output streamed to
// jobs/<id>.log and the event bus, and cancellation via context.
package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Runner satisfies api.JobService structurally (verified where it is wired
// to api.Deps.Jobs in cmd/valheim-ui). It deliberately does not import
// internal/api itself: internal/api's own internal test files import
// internal/instance (WP-02, for a real Service in handler tests), and
// internal/instance (WP-05) depends on this package, so this package
// importing internal/api back would be an import cycle for those test files.

// DefaultMaxConcurrent is how many jobs may run at once across all queues
// when no Option overrides it (ARCHITECTURE.md §9: "At most 4 workers run
// concurrently").
const DefaultMaxConcurrent = 4

// Spec describes a job to enqueue.
type Spec struct {
	Type        domain.JobType
	InstanceID  string // "" for global jobs (e.g. thunderstore_refresh)
	Title       string
	RequestedBy string
	// Exclusive rejects the enqueue with a conflict error when the instance
	// (or, for InstanceID == "", the global queue) already has an active
	// (queued or running) job, instead of queuing behind it. Every job for a
	// given instance already runs serially regardless of this flag; Exclusive
	// additionally refuses to pile up a second job behind an active one.
	Exclusive bool
}

// Func is the body of a job. It must honour ctx for cancellation and use log
// for all output (never write to stdout/stderr directly).
type Func func(ctx context.Context, log *Logger) error

// Option configures a Runner at construction time.
type Option func(*Runner)

// WithMaxConcurrent overrides DefaultMaxConcurrent. Tests use a small number
// to make the concurrency cap observable without long sleeps.
func WithMaxConcurrent(n int) Option {
	return func(r *Runner) {
		if n > 0 {
			r.maxConcurrent = n
		}
	}
}

// Runner runs jobs: one goroutine per active instance queue, capped overall
// by a semaphore of size maxConcurrent, persisted to the jobs table.
type Runner struct {
	db      *sql.DB
	bus     domain.Publisher
	jobsDir string
	log     *slog.Logger

	maxConcurrent int
	sem           chan struct{}

	// baseCtx is the parent of every job's context; cancelling it (on
	// Start's ctx being done) cancels every running job. It is independent
	// of Start being called yet, so Enqueue works immediately after New.
	baseCtx    context.Context
	baseCancel context.CancelFunc

	mu     sync.Mutex
	queues map[string]*jobQueue
	jobs   map[string]*jobRecord

	wg sync.WaitGroup

	started bool
}

// New builds a Runner. jobsDir is created (0750) if missing; a failure to
// create it is logged but not fatal (Enqueue will report it when a job
// actually tries to open its log). Call Start to recover any jobs left
// running/queued by a previous process and to run until ctx is cancelled;
// Enqueue may be called before or after Start.
func New(db *sql.DB, bus domain.Publisher, jobsDir string, log *slog.Logger, opts ...Option) *Runner {
	if log == nil {
		log = slog.Default()
	}
	if err := os.MkdirAll(jobsDir, 0o750); err != nil {
		log.Warn("jobs: create jobs dir", "dir", jobsDir, "err", err)
	}
	baseCtx, cancel := context.WithCancel(context.Background())
	r := &Runner{
		db:            db,
		bus:           bus,
		jobsDir:       jobsDir,
		log:           log,
		maxConcurrent: DefaultMaxConcurrent,
		baseCtx:       baseCtx,
		baseCancel:    cancel,
		queues:        map[string]*jobQueue{},
		jobs:          map[string]*jobRecord{},
	}
	for _, o := range opts {
		o(r)
	}
	r.sem = make(chan struct{}, r.maxConcurrent)
	return r
}

// Start recovers rows left running/queued by a previous process (marking
// them failed with error "manager restarted") and then blocks, cancelling
// every in-flight job's context when ctx is done, until all of them have
// returned. It returns nil on a clean ctx-cancelled shutdown.
func (r *Runner) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return errors.New("jobs: runner already started")
	}
	r.started = true
	r.mu.Unlock()

	stale, err := r.recoverStaleJobs(ctx)
	if err != nil {
		return fmt.Errorf("jobs: recover stale jobs: %w", err)
	}
	for _, rj := range stale {
		r.log.Warn("jobs: marked stale job failed", "job", rj.id, "instance", rj.instanceID, "type", rj.typ)
		r.bus.Publish(domain.Event{
			Name:       domain.EventJobUpdated,
			InstanceID: rj.instanceID,
			Data: domain.Job{
				ID: rj.id, Type: rj.typ, InstanceID: rj.instanceID, Status: domain.JobFailed,
				Title: rj.title, RequestedBy: rj.requestedBy, Error: "manager restarted",
			},
		})
	}

	<-ctx.Done()
	r.baseCancel()
	r.wg.Wait()
	return nil
}

// jobPhase tracks a job's lifecycle from the runner's point of view.
type jobPhase int

const (
	phaseQueued jobPhase = iota
	phaseRunning
	phaseDone
)

// jobRecord is the in-process control block for one job: the persisted Job
// plus everything needed to run and cancel it. Kept for the lifetime of the
// process (bounded by how many jobs are ever enqueued, same as the jobs
// table itself, which is never pruned either).
type jobRecord struct {
	mu    sync.Mutex
	job   domain.Job
	phase jobPhase

	spec   Spec
	fn     Func
	cancel context.CancelFunc

	cancelRequested bool
	done            chan struct{}
}

func (rec *jobRecord) snapshot() *domain.Job {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	j := rec.job
	return &j
}

// jobQueue is one instance's (or the global "") serial FIFO of pending job IDs.
type jobQueue struct {
	mu      sync.Mutex
	pending []string
	active  bool
}

func (r *Runner) getQueue(instanceID string) *jobQueue {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.queues[instanceID]
	if !ok {
		q = &jobQueue{}
		r.queues[instanceID] = q
	}
	return q
}

// ActiveFor returns the oldest queued or running job for instanceID, or nil.
func (r *Runner) ActiveFor(instanceID string) *domain.Job {
	r.mu.Lock()
	recs := make([]*jobRecord, 0, len(r.jobs))
	for _, rec := range r.jobs {
		recs = append(recs, rec)
	}
	r.mu.Unlock()

	var best *domain.Job
	for _, rec := range recs {
		j := rec.snapshot()
		if j.InstanceID != instanceID {
			continue
		}
		if j.Status != domain.JobQueued && j.Status != domain.JobRunning {
			continue
		}
		if best == nil || j.CreatedAt.Before(best.CreatedAt) {
			best = j
		}
	}
	return best
}

// Enqueue persists and schedules a new job. The Func runs on its own
// goroutine once its instance's queue reaches it and a global concurrency
// slot is free.
func (r *Runner) Enqueue(ctx context.Context, spec Spec, fn Func) (*domain.Job, error) {
	if spec.Type == "" {
		return nil, domain.E(domain.CodeValidationFailed, "job type is required")
	}
	if fn == nil {
		return nil, domain.E(domain.CodeInternal, "job function is required")
	}
	if spec.Exclusive {
		if active := r.ActiveFor(spec.InstanceID); active != nil {
			return nil, domain.Ef(domain.CodeInstanceBusy, "a %s job is already active for this instance", active.Type)
		}
	}

	now := time.Now().UTC()
	job := domain.Job{
		ID:          uuid.NewString(),
		Type:        spec.Type,
		InstanceID:  spec.InstanceID,
		Status:      domain.JobQueued,
		Title:       spec.Title,
		RequestedBy: spec.RequestedBy,
		CreatedAt:   now,
	}
	if err := r.insertJob(ctx, job); err != nil {
		return nil, fmt.Errorf("jobs: enqueue: %w", err)
	}

	rec := &jobRecord{job: job, phase: phaseQueued, spec: spec, fn: fn, done: make(chan struct{})}
	r.mu.Lock()
	r.jobs[job.ID] = rec
	r.mu.Unlock()

	r.publish(rec)
	r.scheduleToQueue(spec.InstanceID, job.ID)

	out := job
	return &out, nil
}

// scheduleToQueue appends jobID to instanceID's queue and starts its worker
// goroutine if it is not already running. See serveQueue for the race-free
// active/inactive handshake.
func (r *Runner) scheduleToQueue(instanceID, jobID string) {
	q := r.getQueue(instanceID)
	q.mu.Lock()
	q.pending = append(q.pending, jobID)
	needStart := !q.active
	if needStart {
		q.active = true
	}
	q.mu.Unlock()
	if needStart {
		r.wg.Add(1)
		go r.serveQueue(instanceID, q)
	}
}

func (r *Runner) serveQueue(instanceID string, q *jobQueue) {
	defer r.wg.Done()
	for {
		q.mu.Lock()
		if len(q.pending) == 0 {
			q.active = false
			q.mu.Unlock()
			return
		}
		id := q.pending[0]
		q.pending = q.pending[1:]
		q.mu.Unlock()

		r.runJob(id)
	}
}

// runJob executes one job to completion (or immediate cancellation if it was
// cancelled while still queued).
func (r *Runner) runJob(id string) {
	r.mu.Lock()
	rec, ok := r.jobs[id]
	r.mu.Unlock()
	if !ok {
		r.log.Error("jobs: unknown job popped from queue", "job", id)
		return
	}

	rec.mu.Lock()
	if rec.phase == phaseDone {
		rec.mu.Unlock()
		return // Cancel() already finished this job while it was queued.
	}
	rec.mu.Unlock()

	select {
	case r.sem <- struct{}{}:
	case <-r.baseCtx.Done():
		// Runner shutting down: still take a turn so a cancelled-while-queued
		// job gets finalised below instead of leaking.
		r.sem <- struct{}{}
	}
	defer func() { <-r.sem }()

	rec.mu.Lock()
	if rec.phase == phaseDone {
		rec.mu.Unlock()
		return // cancelled while we waited for a concurrency slot
	}
	jobCtx, cancel := context.WithCancel(r.baseCtx)
	rec.phase = phaseRunning
	rec.cancel = cancel
	cancelAlready := rec.cancelRequested
	rec.mu.Unlock()
	if cancelAlready {
		cancel()
	}

	logger, err := newLogger(r.jobsDir, id, rec.spec.InstanceID, r.bus)
	if err != nil {
		cancel()
		r.finish(rec, domain.JobFailed, fmt.Sprintf("open job log: %v", err), nil)
		return
	}

	startedAt := time.Now().UTC()
	r.markRunning(rec, startedAt)

	runErr := r.invoke(rec, jobCtx, logger)
	cancel()
	summary := logger.summarySnapshot()
	if closeErr := logger.Close(); closeErr != nil {
		r.log.Warn("jobs: close job log", "job", id, "err", closeErr)
	}

	rec.mu.Lock()
	cancelled := rec.cancelRequested
	rec.mu.Unlock()

	status := domain.JobSucceeded
	errMsg := ""
	switch {
	case cancelled || errors.Is(runErr, context.Canceled):
		status = domain.JobCancelled
		if runErr != nil {
			errMsg = runErr.Error()
		}
	case runErr != nil:
		status = domain.JobFailed
		errMsg = runErr.Error()
	}
	r.finish(rec, status, errMsg, summary)
}

// invoke runs fn, converting a panic into an error so one bad job can never
// take down the manager process.
func (r *Runner) invoke(rec *jobRecord, ctx context.Context, logger *Logger) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("job panicked: %v", p)
			r.log.Error("jobs: job panicked", "job", rec.job.ID, "panic", p)
		}
	}()
	return rec.fn(ctx, logger)
}

// Cancel cancels a queued or running job. A queued job is marked cancelled
// immediately without ever running; a running job's context is cancelled and
// it transitions to cancelled once its Func returns (whether it returns nil
// or an error wrapping the cancellation).
func (r *Runner) Cancel(ctx context.Context, id string) (*domain.Job, error) {
	r.mu.Lock()
	rec, ok := r.jobs[id]
	r.mu.Unlock()
	if !ok {
		job, err := r.getJobFromDB(ctx, id)
		if err != nil {
			return nil, err
		}
		return job, nil // job predates this process; nothing left to cancel
	}

	rec.mu.Lock()
	switch rec.phase {
	case phaseDone:
		j := rec.job
		rec.mu.Unlock()
		return &j, nil
	case phaseRunning:
		rec.cancelRequested = true
		cancel := rec.cancel
		j := rec.job
		rec.mu.Unlock()
		cancel()
		return &j, nil
	default: // phaseQueued
		rec.cancelRequested = true
		rec.phase = phaseDone
		rec.mu.Unlock()
	}

	r.removeFromQueue(rec.spec.InstanceID, id)
	r.finish(rec, domain.JobCancelled, "", nil)
	return rec.snapshot(), nil
}

func (r *Runner) removeFromQueue(instanceID, id string) {
	q := r.getQueue(instanceID)
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, pid := range q.pending {
		if pid == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			return
		}
	}
}

// markRunning persists and publishes the queued -> running transition.
func (r *Runner) markRunning(rec *jobRecord, startedAt time.Time) {
	if err := r.updateJobRunning(context.Background(), rec.job.ID, startedAt); err != nil {
		r.log.Error("jobs: persist running", "job", rec.job.ID, "err", err)
	}
	rec.mu.Lock()
	rec.job.Status = domain.JobRunning
	rec.job.StartedAt = &startedAt
	rec.mu.Unlock()
	r.publish(rec)
}

// finish persists and publishes a terminal transition, then unblocks WaitFor.
func (r *Runner) finish(rec *jobRecord, status domain.JobStatus, errMsg string, summary map[string]any) {
	finishedAt := time.Now().UTC()
	if err := r.updateJobFinished(context.Background(), rec.job.ID, status, finishedAt, errMsg, summary); err != nil {
		r.log.Error("jobs: persist finished", "job", rec.job.ID, "err", err)
	}
	rec.mu.Lock()
	rec.job.Status = status
	rec.job.FinishedAt = &finishedAt
	rec.job.Error = errMsg
	rec.job.Summary = summary
	rec.phase = phaseDone
	rec.mu.Unlock()
	r.publish(rec)
	close(rec.done)
}

func (r *Runner) publish(rec *jobRecord) {
	if r.bus == nil {
		return
	}
	j := rec.snapshot()
	r.bus.Publish(domain.Event{Name: domain.EventJobUpdated, InstanceID: j.InstanceID, Data: *j})
}

// Get returns one job by id.
func (r *Runner) Get(ctx context.Context, id string) (*domain.Job, error) {
	return r.getJobFromDB(ctx, id)
}

// List returns jobs newest-first. instanceID == "" or status == "" mean "no
// filter"; limit <= 0 means "no limit".
func (r *Runner) List(ctx context.Context, instanceID string, status domain.JobStatus, limit int) ([]domain.Job, error) {
	return r.listJobsFromDB(ctx, instanceID, status, limit)
}

// Log returns the job's output lines. A job that never started (cancelled
// while queued) has no log file and returns an empty slice, not an error.
func (r *Runner) Log(ctx context.Context, id string) ([]string, error) {
	lines, err := readLogFile(r.jobsDir, id)
	if err == nil {
		return lines, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("jobs: read log: %w", err)
	}
	if _, getErr := r.getJobFromDB(ctx, id); getErr != nil {
		return nil, getErr
	}
	return []string{}, nil
}

// WaitFor blocks until the job reaches a terminal state (or ctx is done) and
// returns its final state. Intended for tests and for callers (e.g. the
// scheduler) that enqueue a job and want to know how it ended.
func (r *Runner) WaitFor(ctx context.Context, id string) (*domain.Job, error) {
	r.mu.Lock()
	rec, ok := r.jobs[id]
	r.mu.Unlock()
	if !ok {
		job, err := r.getJobFromDB(ctx, id)
		if err != nil {
			return nil, err
		}
		if job.Status.Terminal() {
			return job, nil
		}
		return nil, domain.NotFound("job")
	}
	select {
	case <-rec.done:
		return rec.snapshot(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
