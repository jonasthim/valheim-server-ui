package instance

import (
	"context"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// fakeSupervisor is an in-memory supervisor.Supervisor for service tests: it
// never spawns a real process, just tracks a Status per instance id and the
// calls made to it.
type fakeSupervisor struct {
	mu        sync.Mutex
	status    map[string]supervisor.Status
	autostart map[string]bool
	startErr  error
	stopErr   error

	startCalls   []string
	stopCalls    []string
	restartCalls []string
}

func newFakeSupervisor() *fakeSupervisor {
	return &fakeSupervisor{status: map[string]supervisor.Status{}, autostart: map[string]bool{}}
}

func (f *fakeSupervisor) Kind() string { return "fake" }

func (f *fakeSupervisor) Start(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls = append(f.startCalls, id)
	if f.startErr != nil {
		return f.startErr
	}
	f.status[id] = supervisor.Status{State: supervisor.StateRunning, PID: 4242, Since: time.Now()}
	return nil
}

func (f *fakeSupervisor) Stop(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls = append(f.stopCalls, id)
	if f.stopErr != nil {
		return f.stopErr
	}
	f.status[id] = supervisor.Status{State: supervisor.StateStopped}
	return nil
}

func (f *fakeSupervisor) Restart(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restartCalls = append(f.restartCalls, id)
	f.status[id] = supervisor.Status{State: supervisor.StateRunning, PID: 9999, Since: time.Now()}
	return nil
}

func (f *fakeSupervisor) Status(ctx context.Context, id string) (supervisor.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.status[id]
	if st.State == "" {
		st.State = supervisor.StateStopped
	}
	st.Autostart = f.autostart[id]
	return st, nil
}

func (f *fakeSupervisor) SetAutostart(ctx context.Context, id string, on bool) error {
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

// fakeBus captures every published event for assertions.
type fakeBus struct {
	mu     sync.Mutex
	events []domain.Event
}

func (b *fakeBus) Publish(ev domain.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}

func (b *fakeBus) all() []domain.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]domain.Event, len(b.events))
	copy(out, b.events)
	return out
}
