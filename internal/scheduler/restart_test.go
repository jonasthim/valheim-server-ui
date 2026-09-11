package scheduler

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

func TestRestartWarnMarks(t *testing.T) {
	tests := []struct {
		name  string
		delay int
		want  []int
	}{
		// A short delay: only the marks that fit, newest first down to 0.
		{"30s", 30, []int{30, 10}},
		{"60s", 60, []int{60, 30, 10}},
		{"2m", 120, []int{120, 60, 30, 10}},
		{"5m", 300, []int{300, 120, 60, 30, 10}},
		{"10m", 600, []int{600, 300, 120, 60, 30, 10}},
		// An odd delay still starts at the delay itself, then standard marks below it.
		{"90s", 90, []int{90, 60, 30, 10}},
		// Tiny delay: just the delay.
		{"5s", 5, []int{5}},
		// Zero/negative: nothing to announce (caller restarts immediately).
		{"zero", 0, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := restartWarnMarks(tc.delay)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("restartWarnMarks(%d) = %v, want %v", tc.delay, got, tc.want)
			}
		})
	}
}

func TestGracefulRestart_WarnsThenRestarts(t *testing.T) {
	sqldb := newTestDB(t)
	inst, sup := newTestInstanceService(t, sqldb)
	createTestInstance(t, inst, "main")
	sup.setState("main", "running")
	runner := jobs.New(sqldb, nil, t.TempDir(), newTestLogger())

	players := newFakePlayers()
	players.set("main", 3)

	var mu sync.Mutex
	var msgs []string
	hooks := Hooks{
		Broadcast: func(_ context.Context, _ /*instanceID*/, message string) error {
			mu.Lock()
			msgs = append(msgs, message)
			mu.Unlock()
			return nil
		},
	}
	svc := New(sqldb, inst, runner, players, hooks, newTestLogger(),
		WithSleep(func(context.Context, time.Duration) error { return nil }))

	job, err := svc.EnqueueRestart(context.Background(), "main", 120, "alice")
	if err != nil {
		t.Fatalf("EnqueueRestart: %v", err)
	}
	final, err := runner.WaitFor(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if final.Status != domain.JobSucceeded {
		t.Fatalf("job %s: %s", final.Status, final.Error)
	}

	if got := sup.restartCallsSnapshot(); len(got) != 1 || got[0] != "main" {
		t.Fatalf("expected one restart of main, got %v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	// 120s delay → marks 120,60,30,10 plus the final "restarting now".
	want := []string{
		"Server restarting in 2 minutes",
		"Server restarting in 1 minute",
		"Server restarting in 30 seconds",
		"Server restarting in 10 seconds",
		"Server restarting now",
	}
	if !reflect.DeepEqual(msgs, want) {
		t.Fatalf("broadcasts = %v, want %v", msgs, want)
	}
}

func TestGracefulRestart_EmptyServerRestartsImmediately(t *testing.T) {
	sqldb := newTestDB(t)
	inst, sup := newTestInstanceService(t, sqldb)
	createTestInstance(t, inst, "main")
	sup.setState("main", "running")
	runner := jobs.New(sqldb, nil, t.TempDir(), newTestLogger())

	players := newFakePlayers()
	players.set("main", 0) // known, nobody online

	var broadcasts int32
	hooks := Hooks{
		Broadcast: func(context.Context, string, string) error {
			atomic.AddInt32(&broadcasts, 1)
			return nil
		},
	}
	svc := New(sqldb, inst, runner, players, hooks, newTestLogger())

	job, err := svc.EnqueueRestart(context.Background(), "main", 300, "alice")
	if err != nil {
		t.Fatalf("EnqueueRestart: %v", err)
	}
	if _, err := runner.WaitFor(context.Background(), job.ID); err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if n := atomic.LoadInt32(&broadcasts); n != 0 {
		t.Fatalf("expected no broadcasts on an empty server, got %d", n)
	}
	if got := sup.restartCallsSnapshot(); len(got) != 1 {
		t.Fatalf("expected one restart, got %v", got)
	}
}
