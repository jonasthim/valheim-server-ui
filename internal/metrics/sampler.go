// Package metrics samples host and per-process CPU and memory usage from
// /proc (Linux only, no cgo, no dependencies). It enriches InstanceStatus for
// running game servers and feeds the dashboard's host tiles.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
// fixed here.
const clkTck = 100.0

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

// Sampler reads /proc and remembers the previous reading per subject so it
// can turn cumulative CPU time into a percentage. Safe for concurrent use.
type Sampler struct {
	procRoot string
	pageSize int64
	now      func() time.Time

	mu       sync.Mutex
	hostPrev *cpuSample
	hostLast domain.HostMetrics
	hostAt   time.Time
	procs    map[int]*procSample
}

// New returns a Sampler reading the real /proc.
func New() *Sampler {
	return &Sampler{procRoot: "/proc", pageSize: int64(os.Getpagesize()), now: time.Now, procs: map[int]*procSample{}}
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
	busy, total, err := s.readCPU()
	if err != nil {
		return out, err
	}
	if s.hostPrev != nil && total > s.hostPrev.total {
		out.CPUPercent = clampPercent(float64(busy-s.hostPrev.busy) / float64(total-s.hostPrev.total) * 100)
	} else if s.hostPrev != nil {
		out.CPUPercent = s.hostLast.CPUPercent
	}
	s.hostPrev = &cpuSample{busy: busy, total: total, at: now}
	if mt, ma, err := s.readMem(); err == nil {
		out.MemTotalBytes = mt
		out.MemUsedBytes = mt - ma
	}
	if l, err := s.readLoad(); err == nil {
		out.LoadAvg1 = l
	}
	s.hostLast = out
	s.hostAt = now
	return out, nil
}

// Prime takes a first host reading so the next Host call can report a real
// CPU percentage. Errors are ignored: a host without /proc simply reports 0.
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
	ticks, err := s.readProcTicks(pid)
	if err != nil {
		delete(s.procs, pid)
		return domain.ProcessMetrics{}, err
	}
	out := domain.ProcessMetrics{}
	if rss, err := s.readProcRSS(pid); err == nil {
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

func (s *Sampler) readCPU() (busy, total uint64, err error) {
	b, err := os.ReadFile(filepath.Join(s.procRoot, "stat"))
	if err != nil {
		return 0, 0, fmt.Errorf("read /proc/stat: %w", err)
	}
	line, _, _ := strings.Cut(string(b), "\n")
	return parseCPULine(line)
}

// parseCPULine parses the aggregate "cpu ..." line of /proc/stat.
func parseCPULine(line string) (busy, total uint64, err error) {
	f := strings.Fields(line)
	if len(f) < 5 || f[0] != "cpu" {
		return 0, 0, errors.New("metrics: unexpected /proc/stat format")
	}
	vals := make([]uint64, 0, len(f)-1)
	for _, x := range f[1:] {
		n, err := strconv.ParseUint(x, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("metrics: parse /proc/stat: %w", err)
		}
		vals = append(vals, n)
	}
	for _, v := range vals {
		total += v
	}
	idle := vals[3]
	if len(vals) > 4 {
		idle += vals[4] // iowait counts as idle
	}
	return total - idle, total, nil
}

func (s *Sampler) readMem() (total, available int64, err error) {
	b, err := os.ReadFile(filepath.Join(s.procRoot, "meminfo"))
	if err != nil {
		return 0, 0, err
	}
	return parseMeminfo(string(b))
}

// parseMeminfo returns MemTotal and MemAvailable in bytes.
func parseMeminfo(text string) (total, available int64, err error) {
	for _, line := range strings.Split(text, "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if key != "MemTotal" && key != "MemAvailable" {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		kb, perr := strconv.ParseInt(f[0], 10, 64)
		if perr != nil {
			return 0, 0, fmt.Errorf("metrics: parse meminfo %s: %w", key, perr)
		}
		if key == "MemTotal" {
			total = kb * 1024
		} else {
			available = kb * 1024
		}
	}
	if total == 0 {
		return 0, 0, errors.New("metrics: MemTotal missing")
	}
	return total, available, nil
}

func (s *Sampler) readLoad() (float64, error) {
	b, err := os.ReadFile(filepath.Join(s.procRoot, "loadavg"))
	if err != nil {
		return 0, err
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0, errors.New("metrics: empty loadavg")
	}
	return strconv.ParseFloat(f[0], 64)
}

func (s *Sampler) readProcTicks(pid int) (uint64, error) {
	b, err := os.ReadFile(filepath.Join(s.procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	return parseProcStat(string(b))
}

// parseProcStat returns utime+stime (clock ticks) from /proc/<pid>/stat. The
// command name is in parentheses and may itself contain spaces or
// parentheses, so fields are counted from the last ')'.
func parseProcStat(text string) (uint64, error) {
	i := strings.LastIndexByte(text, ')')
	if i < 0 {
		return 0, errors.New("metrics: unexpected /proc/pid/stat format")
	}
	f := strings.Fields(text[i+1:])
	// After ')' the fields are: state(3) ppid pgrp session tty tpgid flags
	// minflt cminflt majflt cmajflt utime(14) stime(15) ...
	if len(f) < 13 {
		return 0, errors.New("metrics: short /proc/pid/stat")
	}
	ut, err := strconv.ParseUint(f[11], 10, 64)
	if err != nil {
		return 0, err
	}
	st, err := strconv.ParseUint(f[12], 10, 64)
	if err != nil {
		return 0, err
	}
	return ut + st, nil
}

func (s *Sampler) readProcRSS(pid int) (int64, error) {
	b, err := os.ReadFile(filepath.Join(s.procRoot, strconv.Itoa(pid), "statm"))
	if err != nil {
		return 0, err
	}
	f := strings.Fields(string(b))
	if len(f) < 2 {
		return 0, errors.New("metrics: short /proc/pid/statm")
	}
	pages, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return pages * s.pageSize, nil
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
