package metrics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func writeProc(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func newTestSampler(t *testing.T) (*Sampler, *time.Time) {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	s := &Sampler{r: procfsReader{root: root, pageSize: 4096}, procs: map[int]*procSample{}}
	s.now = func() time.Time { return now }
	return s, &now
}

// procRoot returns the fixture directory a test sampler reads as /proc.
func procRoot(s *Sampler) string { return s.r.(procfsReader).root }

func TestParsers(t *testing.T) {
	busy, total, err := parseCPULine("cpu  100 0 50 800 50 0 0 0 0 0")
	if err != nil || busy != 150 || total != 1000 {
		t.Fatalf("parseCPULine = %d %d %v", busy, total, err)
	}
	if _, _, err := parseCPULine("intr 1 2 3"); err == nil {
		t.Fatal("expected error for a non-cpu line")
	}
	mt, ma, err := parseMeminfo("MemTotal:       16461028 kB\nMemFree: 1 kB\nMemAvailable:   15663584 kB\n")
	if err != nil || mt != 16461028*1024 || ma != 15663584*1024 {
		t.Fatalf("parseMeminfo = %d %d %v", mt, ma, err)
	}
	// Command names may contain spaces and parentheses.
	ticks, err := parseProcStat("4242 (valheim (x64) srv) S 1 4242 4242 0 -1 4194560 100 0 0 0 250 50 0 0 20 0 30 0 100 1 2 3\n")
	if err != nil || ticks != 300 {
		t.Fatalf("parseProcStat = %d %v", ticks, err)
	}
}

func TestHostCPUPercentNeedsTwoReadings(t *testing.T) {
	s, now := newTestSampler(t)
	writeProc(t, procRoot(s), map[string]string{
		"stat":    "cpu  100 0 50 800 50 0 0 0 0 0\n",
		"meminfo": "MemTotal: 1000 kB\nMemAvailable: 250 kB\n",
		"loadavg": "0.42 0.5 0.6 1/10 99\n",
	})
	h, err := s.Host()
	if err != nil {
		t.Fatal(err)
	}
	if h.CPUPercent != 0 || h.MemTotalBytes != 1000*1024 || h.MemUsedBytes != 750*1024 || h.LoadAvg1 != 0.42 {
		t.Fatalf("first reading = %+v", h)
	}
	// 2 s later: 200 more ticks total, 100 of them busy -> 50 %.
	*now = now.Add(2 * time.Second)
	writeProc(t, procRoot(s), map[string]string{"stat": "cpu  200 0 50 900 50 0 0 0 0 0\n"})
	h, err = s.Host()
	if err != nil {
		t.Fatal(err)
	}
	if h.CPUPercent != 50 {
		t.Fatalf("cpu = %v, want 50", h.CPUPercent)
	}
	// Within minInterval the cached figure is returned even if /proc moved on.
	*now = now.Add(200 * time.Millisecond)
	writeProc(t, procRoot(s), map[string]string{"stat": "cpu  900 0 50 900 50 0 0 0 0 0\n"})
	if h, _ = s.Host(); h.CPUPercent != 50 {
		t.Fatalf("cached cpu = %v, want 50", h.CPUPercent)
	}
}

func TestProcessAndEnrich(t *testing.T) {
	s, now := newTestSampler(t)
	writeProc(t, procRoot(s), map[string]string{
		"4242/stat":  "4242 (valheim_server) S 1 1 1 0 -1 0 0 0 0 0 100 100 0 0 20 0 30 0 100 1 2 3\n",
		"4242/statm": "5000 2560 300 1 0 100 0\n",
	})
	st := &domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning, PID: 4242}
	s.Enrich(context.Background(), st)
	if st.MemoryBytes == nil || *st.MemoryBytes != 2560*4096 {
		t.Fatalf("memory = %v, want %d", st.MemoryBytes, 2560*4096)
	}
	if st.CPUPercent != nil {
		t.Fatalf("cpu should be unknown on the first reading, got %v", *st.CPUPercent)
	}

	// 4 s later the process used 200 more ticks = 2 s of CPU -> 50 % of a core.
	*now = now.Add(4 * time.Second)
	writeProc(t, procRoot(s), map[string]string{
		"4242/stat": "4242 (valheim_server) S 1 1 1 0 -1 0 0 0 0 0 250 150 0 0 20 0 30 0 100 1 2 3\n",
	})
	st = &domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning, PID: 4242}
	s.Enrich(context.Background(), st)
	if st.CPUPercent == nil || *st.CPUPercent != 50 {
		t.Fatalf("cpu = %v, want 50", st.CPUPercent)
	}

	// Stopped instances and unknown pids are left untouched.
	stopped := &domain.InstanceStatus{InstanceID: "main", State: domain.StateStopped, PID: 4242}
	s.Enrich(context.Background(), stopped)
	if stopped.CPUPercent != nil || stopped.MemoryBytes != nil {
		t.Fatalf("stopped instance got metrics: %+v", stopped)
	}
	gone := &domain.InstanceStatus{InstanceID: "x", State: domain.StateRunning, PID: 9999}
	s.Enrich(context.Background(), gone)
	if gone.MemoryBytes != nil {
		t.Fatalf("missing pid got metrics: %+v", gone)
	}
}
