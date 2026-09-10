// Package metrics samples host and per-process CPU and memory usage: from
// /proc on Linux, from the Win32 process and memory APIs on Windows (no cgo).
// It enriches InstanceStatus for running game servers and feeds the
// dashboard's host tiles.
package metrics

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// minInterval is the shortest window a CPU percentage is computed over; two
// requests closer together than this reuse the previous figure. It keeps the
// number stable when several clients poll at once.
const minInterval = time.Second

// procTTL is how long a process sample is kept after it was last requested.
const procTTL = 5 * time.Minute

// clkTck is USER_HZ, the unit of the CPU time fields in /proc. It is 100 on
// every mainstream Linux build; reading it needs cgo (sysconf), so it is
// fixed here. The Windows reader converts its 100 ns process times to the
// same 10 ms ticks.
const clkTck = 100.0

// reader is the platform source of raw counters (procfsReader, winReader).
type reader interface {
	cpu() (busy, total uint64, err error)
	mem() (total, available int64, err error)
	load() (float64, error)
	procTicks(pid int) (uint64, error)
	procRSS(pid int) (int64, error)
}

type cpuSample struct {
	busy, total uint64
	at          time.Time
}

type procSample struct {
	ticks   uint64
	at      time.Time
	last    domain.ProcessMetrics
	seen    time.Time
	primed  bool
	sampled bool
}

// Sampler reads the platform counters and remembers the previous reading
// per subject so it can turn cumulative CPU time into a percentage. Safe for
// concurrent use.
type Sampler struct {
	r   reader
	now func() time.Time

	mu       sync.Mutex
	hostPrev *cpuSample
	hostLast domain.HostMetrics
	hostAt   time.Time
	procs    map[int]*procSample
}

// New returns a Sampler reading the live system.
func New() *Sampler {
	return &Sampler{r: newReader(), now: time.Now, procs: map[int]*procSample{}}
}

// Host returns CPU utilisation across all cores (0-100), the 1-minute load
// average, core count and memory totals. The first call after start reports
// 0 % CPU because a percentage needs two readings; call Prime at startup.
func (s *Sampler) Host() (domain.HostMetrics, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !s.hostAt.IsZero() && now.Sub(s.hostAt) < minInterval {
		return s.hostLast, nil
	}
	out := domain.HostMetrics{CPUCount: runtime.NumCPU()}
	busy, total, err := s.r.cpu()
	if err != nil {
		return out, err
	}
	if s.hostPrev != nil && total > s.hostPrev.total {
		out.CPUPercent = clampPercent(float64(busy-s.hostPrev.busy) / float64(total-s.hostPrev.total) * 100)
	} else if s.hostPrev != nil {
		out.CPUPercent = s.hostLast.CPUPercent
	}
	s.hostPrev = &cpuSample{busy: busy, total: total, at: now}
	if mt, ma, err := s.r.mem(); err == nil {
		out.MemTotalBytes = mt
		out.MemUsedBytes = mt - ma
	}
	if l, err := s.r.load(); err == nil {
		out.LoadAvg1 = l
	}
	s.hostLast = out
	s.hostAt = now
	return out, nil
}

// Prime takes a first host reading so the next Host call can report a real
// CPU percentage. Errors are ignored: a host without counters simply reports 0.
func (s *Sampler) Prime() { _, _ = s.Host() }

// Process returns the resident memory and CPU usage (percent of one core) of
// pid. CPUKnown is false on the first reading of a pid.
func (s *Sampler) Process(pid int) (domain.ProcessMetrics, error) {
	if pid <= 0 {
		return domain.ProcessMetrics{}, errors.New("metrics: invalid pid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.evictLocked(now)
	ps, ok := s.procs[pid]
	if !ok {
		ps = &procSample{}
		s.procs[pid] = ps
	}
	ps.seen = now
	if ps.sampled && now.Sub(ps.at) < minInterval {
		return ps.last, nil
	}
	ticks, err := s.r.procTicks(pid)
	if err != nil {
		delete(s.procs, pid)
		return domain.ProcessMetrics{}, err
	}
	out := domain.ProcessMetrics{}
	if rss, err := s.r.procRSS(pid); err == nil {
		out.MemoryBytes = rss
	}
	if ps.primed {
		if dt := now.Sub(ps.at).Seconds(); dt > 0 && ticks >= ps.ticks {
			out.CPUPercent = float64(ticks-ps.ticks) / clkTck / dt * 100
			out.CPUKnown = true
		}
	}
	ps.ticks, ps.at, ps.primed, ps.sampled, ps.last = ticks, now, true, true, out
	return out, nil
}

// Enrich implements domain.StatusEnricher: running instances get CPU and
// memory figures for their game process.
func (s *Sampler) Enrich(_ context.Context, st *domain.InstanceStatus) {
	if st == nil || st.PID <= 0 || (st.State != domain.StateRunning && st.State != domain.StateStarting) {
		return
	}
	pm, err := s.Process(st.PID)
	if err != nil {
		return
	}
	mem := pm.MemoryBytes
	st.MemoryBytes = &mem
	if pm.CPUKnown {
		cpu := pm.CPUPercent
		st.CPUPercent = &cpu
	}
}

func (s *Sampler) evictLocked(now time.Time) {
	for pid, ps := range s.procs {
		if now.Sub(ps.seen) > procTTL {
			delete(s.procs, pid)
		}
	}
}

func clampPercent(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 100:
		return 100
	default:
		return v
	}
}
