package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// fakeSupervisor is a minimal in-memory supervisor.Supervisor, mirroring the
// pattern in internal/instance/service_test.go, for building a real
// *instance.Service in scheduler tests without spawning any process.
type fakeSupervisor struct {
	mu           sync.Mutex
	status       map[string]supervisor.Status
	restartCalls []string
}

func newFakeSupervisor() *fakeSupervisor {
	return &fakeSupervisor{status: map[string]supervisor.Status{}}
}

func (f *fakeSupervisor) Kind() string { return "fake" }

func (f *fakeSupervisor) Start(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = supervisor.Status{State: supervisor.StateRunning, PID: 100, Since: time.Now()}
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
	f.restartCalls = append(f.restartCalls, id)
	f.status[id] = supervisor.Status{State: supervisor.StateRunning, PID: 200, Since: time.Now()}
	return nil
}

// restartCallsSnapshot returns a copy of the instance ids Restart was called
// with, safe to read concurrently with the supervisor still in use.
func (f *fakeSupervisor) restartCallsSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.restartCalls))
	copy(out, f.restartCalls)
	return out
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

// setState lets a test force a supervisor state directly (e.g. "running")
// without going through Start.
func (f *fakeSupervisor) setState(id string, st supervisor.State) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.status[id]
	s.State = st
	f.status[id] = s
}

// fakePlayers is a domain.PlayerCounter test double: PlayersOnline for an
// instance id not registered via set reports "unknown" (0, false), matching
// a real PlayerCounter's answer before it has ever seen the instance.
type fakePlayers struct {
	mu     sync.Mutex
	counts map[string]int
	known  map[string]bool
}

func newFakePlayers() *fakePlayers {
	return &fakePlayers{counts: map[string]int{}, known: map[string]bool{}}
}

func (f *fakePlayers) set(instanceID string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[instanceID] = n
	f.known[instanceID] = true
}

func (f *fakePlayers) PlayersOnline(_ context.Context, instanceID string) (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.known[instanceID] {
		return 0, false
	}
	return f.counts[instanceID], true
}

// memDBCounter gives every test its own uniquely-named SQLite shared-cache
// in-memory database. db.OpenMemory always uses the same fixed DSN
// ("file::memory:?cache=shared"), which under SQLite's shared-cache mode
// means every *sql.DB that uses it for as long as any one of them still has
// a connection open shares the very same database — including one a
// previous test's background goroutine (e.g. waitAndRecordFailure) hasn't
// finished with yet when the next test starts. A unique name per test avoids
// that cross-test bleed entirely, independent of goroutine timing.
var memDBCounter int64

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("scheduler_test_%d", atomic.AddInt64(&memDBCounter, 1))
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", name)
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	sqldb.SetMaxOpenConns(1)
	if err := db.Migrate(context.Background(), sqldb); err != nil {
		t.Fatalf("migrate memory db: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	return sqldb
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestInstanceService builds a real *instance.Service backed by sqldb and
// a fakeSupervisor, so the scheduler's restart path (Status/Restart/Exists)
// exercises real code without a game process.
func newTestInstanceService(t *testing.T, sqldb *sql.DB) (*instance.Service, *fakeSupervisor) {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Supervisor = "direct"
	sup := newFakeSupervisor()
	return instance.New(sqldb, nil, sup, cfg, newTestLogger()), sup
}

var testInstancePort int32 = 2456

// createTestInstance inserts a valid instance row, using a fresh port each
// call so multiple instances can coexist in one test's database.
func createTestInstance(t *testing.T, inst *instance.Service, id string) {
	t.Helper()
	port := int(atomic.AddInt32(&testInstancePort, 4))
	cfg := domain.InstanceConfig{Name: "Test " + id, World: "Dedicated", Password: "secret123", Port: port, Public: true}
	cfg.ApplyDefaults()
	if _, err := inst.Create(context.Background(), id, "Test "+id, cfg, false); err != nil {
		t.Fatalf("create instance %s: %v", id, err)
	}
}

func requireDomainError(t *testing.T, err error) *domain.Error {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	return domain.AsError(err)
}
