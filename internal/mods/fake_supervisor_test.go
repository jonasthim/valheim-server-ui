package mods

import (
	"context"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// fakeBus is a no-op domain.Publisher for tests that need to hand *something*
// to instance.New/jobs.New but do not assert on published events.
type fakeBus struct{}

func (fakeBus) Publish(domain.Event) {}

// fakeSupervisor is an in-memory supervisor.Supervisor for service tests
// (copied from internal/instance/fake_supervisor_test.go per WORKPLAN.md
// WP-08's test plan): it never spawns a real process, just tracks a Status
// per instance id.
type fakeSupervisor struct {
	mu        sync.Mutex
	status    map[string]supervisor.Status
	autostart map[string]bool
}

func newFakeSupervisor() *fakeSupervisor {
	return &fakeSupervisor{status: map[string]supervisor.Status{}, autostart: map[string]bool{}}
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
	f.status[id] = supervisor.Status{State: supervisor.StateStopped}
	return nil
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
	st.Autostart = f.autostart[id]
	return st, nil
}

func (f *fakeSupervisor) SetAutostart(_ context.Context, id string, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.autostart[id] = on
	return nil
}

// setStatus lets tests force a supervisor state (e.g. "running") without
// going through Start.
func (f *fakeSupervisor) setStatus(id string, st supervisor.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = st
}
