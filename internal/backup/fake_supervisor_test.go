package backup

import (
	"context"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// fakeSupervisor is an in-memory supervisor.Supervisor for backup service
// tests: it never spawns a real process, just tracks a Status per instance
// id (mirrors internal/instance/fake_supervisor_test.go, copied here since
// that one is unexported to its own package's test binary).
type fakeSupervisor struct {
	mu      sync.Mutex
	status  map[string]supervisor.Status
	stopped []string
}

func newFakeSupervisor() *fakeSupervisor {
	return &fakeSupervisor{status: map[string]supervisor.Status{}}
}

func (f *fakeSupervisor) Kind() string { return "fake" }

func (f *fakeSupervisor) Start(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = supervisor.Status{State: supervisor.StateRunning, PID: 4242, Since: time.Now()}
	return nil
}

func (f *fakeSupervisor) Stop(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, id)
	f.status[id] = supervisor.Status{State: supervisor.StateStopped}
	return nil
}

// stopCalls returns the instance ids Stop was called with, in order.
func (f *fakeSupervisor) stopCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.stopped))
	copy(out, f.stopped)
	return out
}

func (f *fakeSupervisor) Restart(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = supervisor.Status{State: supervisor.StateRunning, PID: 9999, Since: time.Now()}
	return nil
}

func (f *fakeSupervisor) Status(_ context.Context, id string) (supervisor.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.status[id]
	if st.State == "" {
		st.State = supervisor.StateStopped
	}
	return st, nil
}

func (f *fakeSupervisor) SetAutostart(_ context.Context, _ string, _ bool) error { return nil }

// setStatus lets tests force a supervisor state (e.g. "running") directly.
func (f *fakeSupervisor) setStatus(id string, st supervisor.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = st
}
