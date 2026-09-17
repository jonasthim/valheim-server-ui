package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// defaultInterval is how often Run samples resource usage, absent WithInterval.
const defaultInterval = 60 * time.Second

// maintenanceHour is the local hour (0-23) at or after which the first tick
// of a new calendar day runs downsample/prune housekeeping.
const maintenanceHour = 3

// downsampleAge/pruneAge bound how much raw and hourly history is kept.
const (
	downsampleAge = 7 * 24 * time.Hour
	pruneAge      = 90 * 24 * time.Hour
)

// InstanceLister lists instances for one sample tick (internal/api.InstanceService).
type InstanceLister interface {
	List(ctx context.Context) ([]domain.Instance, error)
}

// HostSource reports the manager host's resource usage (Sampler.Host).
type HostSource interface {
	Host() (domain.HostMetrics, error)
}

// SampleStore persists metric samples (internal/db.MetricSamplesRepo).
type SampleStore interface {
	Insert(ctx context.Context, s domain.MetricSample) error
	Downsample(ctx context.Context, olderThan time.Time) error
	Prune(ctx context.Context, olderThan time.Time) error
}

// Recorder samples every running instance plus the host on a fixed interval
// and writes them to a SampleStore, ageing out old history once a day
// (F-1.3). A zero Recorder is not usable; construct with NewRecorder.
type Recorder struct {
	store     SampleStore
	instances InstanceLister
	host      HostSource // may be nil: system endpoints without metrics support
	dataDir   string
	log       *slog.Logger

	interval time.Duration
	now      func() time.Time

	mu              sync.Mutex
	lastMaintenance string // "2006-01-02" of the last successful maintenance run, "" = never
}

// RecorderOption configures a Recorder at construction time.
type RecorderOption func(*Recorder)

// WithInterval overrides the sample interval (tests only; production keeps
// the 60s default).
func WithInterval(d time.Duration) RecorderOption {
	return func(r *Recorder) {
		if d > 0 {
			r.interval = d
		}
	}
}

// WithClock overrides time.Now (tests only).
func WithClock(fn func() time.Time) RecorderOption {
	return func(r *Recorder) {
		if fn != nil {
			r.now = fn
		}
	}
}

// NewRecorder builds a Recorder. Call Run to start the sampling loop. host
// may be nil when the manager has no metrics sampler configured.
func NewRecorder(store SampleStore, instances InstanceLister, host HostSource, dataDir string, log *slog.Logger, opts ...RecorderOption) *Recorder {
	if log == nil {
		log = slog.Default()
	}
	r := &Recorder{
		store:     store,
		instances: instances,
		host:      host,
		dataDir:   dataDir,
		log:       log,
		interval:  defaultInterval,
		now:       time.Now,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Run samples on a ticker until ctx is done. A sample or housekeeping error
// is logged and never stops the loop.
func (r *Recorder) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

// tick records one sample per running instance plus one host sample, then
// runs daily maintenance if it is due. Errors are logged, never returned:
// one bad tick must not stop the loop.
func (r *Recorder) tick(ctx context.Context) {
	now := r.now()

	instances, err := r.instances.List(ctx)
	if err != nil {
		r.log.Warn("metrics: list instances", "err", err)
	}

	playersRunning := 0
	for _, inst := range instances {
		if inst.Status.State != domain.StateRunning {
			continue
		}
		playersRunning += inst.Status.PlayersOnline

		var cpu float64
		if inst.Status.CPUPercent != nil {
			cpu = *inst.Status.CPUPercent
		}
		var mem int64
		if inst.Status.MemoryBytes != nil {
			mem = *inst.Status.MemoryBytes
		}
		id := inst.ID
		s := domain.MetricSample{
			InstanceID: &id,
			At:         now,
			CPU:        cpu,
			Mem:        mem,
			Players:    inst.Status.PlayersOnline,
		}
		if err := r.store.Insert(ctx, s); err != nil {
			r.log.Warn("metrics: insert instance sample", "instance", inst.ID, "err", err)
		}
	}

	if r.host != nil {
		r.recordHost(ctx, now, playersRunning)
	}

	r.maybeMaintain(ctx, now)
}

func (r *Recorder) recordHost(ctx context.Context, now time.Time, playersRunning int) {
	h, err := r.host.Host()
	if err != nil {
		r.log.Warn("metrics: host metrics", "err", err)
		return
	}
	free, _, err := DiskUsage(r.dataDir)
	if err != nil {
		r.log.Warn("metrics: disk usage", "data_dir", r.dataDir, "err", err)
		free = 0
	}
	s := domain.MetricSample{
		InstanceID: nil,
		At:         now,
		CPU:        h.CPUPercent,
		Mem:        h.MemUsedBytes,
		Players:    playersRunning,
		DiskFree:   free,
	}
	if err := r.store.Insert(ctx, s); err != nil {
		r.log.Warn("metrics: insert host sample", "err", err)
	}
}

// maybeMaintain runs Downsample/Prune once on the first tick whose hour (as
// reported by r.now, which is the local wall clock in production) is at or
// after maintenanceHour on a calendar day not yet maintained.
func (r *Recorder) maybeMaintain(ctx context.Context, now time.Time) {
	if now.Hour() < maintenanceHour {
		return
	}
	today := now.Format("2006-01-02")

	r.mu.Lock()
	if r.lastMaintenance == today {
		r.mu.Unlock()
		return
	}
	r.lastMaintenance = today
	r.mu.Unlock()

	if err := r.store.Downsample(ctx, now.Add(-downsampleAge)); err != nil {
		r.log.Warn("metrics: downsample", "err", err)
	}
	if err := r.store.Prune(ctx, now.Add(-pruneAge)); err != nil {
		r.log.Warn("metrics: prune", "err", err)
	}
}
