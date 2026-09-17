package metrics

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

type fakeInstanceLister struct {
	instances []domain.Instance
	err       error
}

func (f *fakeInstanceLister) List(_ context.Context) ([]domain.Instance, error) {
	return f.instances, f.err
}

type fakeHostSource struct {
	host domain.HostMetrics
	err  error
}

func (f *fakeHostSource) Host() (domain.HostMetrics, error) { return f.host, f.err }

// recordingStore is a fake SampleStore that records every call, for
// assertions without a real database.
type recordingStore struct {
	mu              sync.Mutex
	inserted        []domain.MetricSample
	downsampleCalls int
	pruneCalls      int
}

func (s *recordingStore) Insert(_ context.Context, sample domain.MetricSample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inserted = append(s.inserted, sample)
	return nil
}

func (s *recordingStore) Downsample(_ context.Context, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downsampleCalls++
	return nil
}

func (s *recordingStore) Prune(_ context.Context, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneCalls++
	return nil
}

func (s *recordingStore) snapshot() (inserted []domain.MetricSample, downsampleCalls, pruneCalls int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.MetricSample(nil), s.inserted...), s.downsampleCalls, s.pruneCalls
}

func cpuPtr(v float64) *float64 { return &v }
func memPtr(v int64) *int64     { return &v }

func TestRecorderTick_InsertsRunningInstancesAndHost(t *testing.T) {
	lister := &fakeInstanceLister{instances: []domain.Instance{
		{ID: "a", Status: domain.InstanceStatus{State: domain.StateRunning, PlayersOnline: 2, CPUPercent: cpuPtr(11), MemoryBytes: memPtr(1000)}},
		{ID: "b", Status: domain.InstanceStatus{State: domain.StateRunning, PlayersOnline: 3, CPUPercent: cpuPtr(22), MemoryBytes: memPtr(2000)}},
		{ID: "c", Status: domain.InstanceStatus{State: domain.StateStopped, PlayersOnline: 0}},
	}}
	host := &fakeHostSource{host: domain.HostMetrics{CPUPercent: 33, MemUsedBytes: 5000}}
	store := &recordingStore{}

	rec := NewRecorder(store, lister, host, t.TempDir(), nil)
	rec.tick(context.Background())

	inserted, _, _ := store.snapshot()
	if len(inserted) != 3 {
		t.Fatalf("expected exactly 3 rows (2 running instances + 1 host), got %d: %+v", len(inserted), inserted)
	}

	var hostSample *domain.MetricSample
	instanceIDs := map[string]bool{}
	for i, s := range inserted {
		if s.InstanceID == nil {
			hostSample = &inserted[i]
			continue
		}
		instanceIDs[*s.InstanceID] = true
	}
	if len(instanceIDs) != 2 || !instanceIDs["a"] || !instanceIDs["b"] {
		t.Fatalf("expected samples for running instances a and b only, got %+v", instanceIDs)
	}
	if hostSample == nil {
		t.Fatal("expected one host sample (nil instance id)")
	}
	if hostSample.CPU != 33 {
		t.Errorf("expected host cpu 33, got %v", hostSample.CPU)
	}
	if hostSample.Mem != 5000 {
		t.Errorf("expected host mem 5000, got %v", hostSample.Mem)
	}
	if hostSample.Players != 5 {
		t.Errorf("expected host players to be the sum over running instances (2+3=5), got %d", hostSample.Players)
	}
}

func TestRecorderTick_NoHostSourceInsertsOnlyInstances(t *testing.T) {
	lister := &fakeInstanceLister{instances: []domain.Instance{
		{ID: "a", Status: domain.InstanceStatus{State: domain.StateRunning}},
	}}
	store := &recordingStore{}

	rec := NewRecorder(store, lister, nil, t.TempDir(), nil)
	rec.tick(context.Background())

	inserted, _, _ := store.snapshot()
	if len(inserted) != 1 || inserted[0].InstanceID == nil {
		t.Fatalf("expected exactly 1 instance row and no host row, got %+v", inserted)
	}
}

func TestRecorderTick_MaintenanceRunsOncePerDayAfter3AM(t *testing.T) {
	lister := &fakeInstanceLister{}
	store := &recordingStore{}

	clock := time.Date(2026, 9, 17, 2, 0, 0, 0, time.UTC)
	rec := NewRecorder(store, lister, nil, t.TempDir(), nil, WithClock(func() time.Time { return clock }))

	// Before 03:00 local: no maintenance yet.
	rec.tick(context.Background())
	if _, ds, pr := store.snapshot(); ds != 0 || pr != 0 {
		t.Fatalf("expected no maintenance before 03:00, got downsample=%d prune=%d", ds, pr)
	}

	// Cross 03:00 on the same calendar day: maintenance runs exactly once.
	clock = time.Date(2026, 9, 17, 3, 5, 0, 0, time.UTC)
	rec.tick(context.Background())
	if _, ds, pr := store.snapshot(); ds != 1 || pr != 1 {
		t.Fatalf("expected maintenance to run once, got downsample=%d prune=%d", ds, pr)
	}

	// Another tick later the same day: not repeated.
	clock = time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	rec.tick(context.Background())
	if _, ds, pr := store.snapshot(); ds != 1 || pr != 1 {
		t.Fatalf("expected maintenance to run only once per day, got downsample=%d prune=%d", ds, pr)
	}

	// The next day past 03:00: maintenance runs again.
	clock = time.Date(2026, 9, 18, 4, 0, 0, 0, time.UTC)
	rec.tick(context.Background())
	if _, ds, pr := store.snapshot(); ds != 2 || pr != 2 {
		t.Fatalf("expected maintenance to run again the next day, got downsample=%d prune=%d", ds, pr)
	}
}

func TestRecorderTick_NeverPanicsOnStoreOrListErrors(t *testing.T) {
	lister := &fakeInstanceLister{err: context.DeadlineExceeded}
	host := &fakeHostSource{err: context.DeadlineExceeded}
	store := &recordingStore{}

	rec := NewRecorder(store, lister, host, t.TempDir(), nil)
	rec.tick(context.Background()) // must not panic

	inserted, _, _ := store.snapshot()
	if len(inserted) != 0 {
		t.Fatalf("expected no rows when List and Host both fail, got %+v", inserted)
	}
}
