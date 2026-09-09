// Package scheduler implements api.ScheduleService and the cron engine
// described in docs/ARCHITECTURE.md §11: schedules are loaded into one
// robfig/cron instance (host location, standard 5-field cron plus
// descriptors), reloaded whenever a schedule changes, and each firing enqueues
// (or skips) the appropriate action — a restart job, a backup, or an update —
// recording last_run_at/last_result/last_job_id.
//
// The scheduler never imports internal/backup or internal/instance/jobs.go
// directly; the concrete backup and update actions are injected as Hooks at
// construction time so this package can be developed and tested against
// those work packages independently.
package scheduler

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// cronParseOptions matches ARCHITECTURE.md §11: standard 5-field cron plus
// @daily/@hourly/... descriptors, no seconds field.
const cronParseOptions = cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor

// Hooks are the concrete actions the scheduler triggers, wired in by
// cmd/valheim-ui/wire_scheduler.go to the backup service (WP-06) and the
// Steam update job (WP-05) without this package importing either.
type Hooks struct {
	// Backup enqueues a scheduled backup (kind "scheduled").
	Backup func(ctx context.Context, instanceID, requestedBy string) (*domain.Job, error)
	// Update enqueues the Steam update job.
	Update func(ctx context.Context, instanceID, requestedBy string, stopIfRunning bool) (*domain.Job, error)
	// UpdateAvailable reports whether a newer Steam build exists.
	UpdateAvailable func(ctx context.Context, instanceID string) (bool, error)
}

// Service implements api.ScheduleService and the cron engine.
type Service struct {
	db      *sql.DB
	inst    *instance.Service
	runner  *jobs.Runner
	players domain.PlayerCounter
	hooks   Hooks
	log     *slog.Logger

	now    func() time.Time
	loc    *time.Location
	parser cron.Parser

	// baseCtx is cancelled when Run's ctx is done, so cron-fired job waits
	// (waitAndRecordFailure) never outlive the scheduler.
	baseCtx    context.Context
	baseCancel context.CancelFunc

	mu      sync.Mutex
	started bool
	cronEng *cron.Cron

	wg sync.WaitGroup
}

// Option configures a Service at construction time.
type Option func(*Service)

// WithClock overrides time.Now, for tests.
func WithClock(fn func() time.Time) Option {
	return func(s *Service) {
		if fn != nil {
			s.now = fn
		}
	}
}

// WithLocation overrides the cron/NextRunAt timezone (defaults to time.Local,
// i.e. the host timezone per ARCHITECTURE.md §11), for tests.
func WithLocation(loc *time.Location) Option {
	return func(s *Service) {
		if loc != nil {
			s.loc = loc
		}
	}
}

// New builds the scheduler service. Call Run to start firing schedules.
func New(db *sql.DB, inst *instance.Service, runner *jobs.Runner, players domain.PlayerCounter, hooks Hooks, log *slog.Logger, opts ...Option) *Service {
	if log == nil {
		log = slog.Default()
	}
	baseCtx, cancel := context.WithCancel(context.Background())
	s := &Service{
		db:         db,
		inst:       inst,
		runner:     runner,
		players:    players,
		hooks:      hooks,
		log:        log,
		now:        time.Now,
		loc:        time.Local,
		parser:     cron.NewParser(cronParseOptions),
		baseCtx:    baseCtx,
		baseCancel: cancel,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}
