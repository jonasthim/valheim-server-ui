package db

import (
	"context"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestMetricSamplesInsertAndRange(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	repo := NewMetricSamplesRepo(sqldb)
	ctx := context.Background()

	instanceID := "a"
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	samples := []domain.MetricSample{
		{InstanceID: &instanceID, At: base, CPU: 10, Mem: 1000, Players: 1, DiskFree: 5000},
		{InstanceID: &instanceID, At: base.Add(1 * time.Minute), CPU: 20, Mem: 2000, Players: 2, DiskFree: 4900},
		{InstanceID: &instanceID, At: base.Add(2 * time.Minute), CPU: 30, Mem: 3000, Players: 3, DiskFree: 4800},
		{InstanceID: nil, At: base, CPU: 5, Mem: 500, Players: 0, DiskFree: 5000},
	}
	for _, s := range samples {
		if err := repo.Insert(ctx, s); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	since := base.Add(-1 * time.Hour)
	got, err := repo.Range(ctx, &instanceID, since)
	if err != nil {
		t.Fatalf("range instance: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 samples for instance a, got %d: %+v", len(got), got)
	}
	for i, want := range []float64{10, 20, 30} {
		if got[i].CPU != want {
			t.Errorf("sample %d: expected cpu %v, got %v", i, want, got[i].CPU)
		}
		if got[i].InstanceID == nil || *got[i].InstanceID != "a" {
			t.Errorf("sample %d: expected instance id \"a\", got %v", i, got[i].InstanceID)
		}
	}
	if !got[0].At.Equal(base) || !got[2].At.Equal(base.Add(2*time.Minute)) {
		t.Errorf("expected ascending order by at, got %+v", got)
	}

	host, err := repo.Range(ctx, nil, since)
	if err != nil {
		t.Fatalf("range host: %v", err)
	}
	if len(host) != 1 {
		t.Fatalf("expected 1 host sample, got %d: %+v", len(host), host)
	}
	if host[0].InstanceID != nil {
		t.Errorf("expected the host sample's instance id to be nil, got %v", *host[0].InstanceID)
	}
	if host[0].CPU != 5 {
		t.Errorf("expected the host sample's cpu to be 5, got %v", host[0].CPU)
	}
}

func TestMetricSamplesDownsample(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	repo := NewMetricSamplesRepo(sqldb)
	ctx := context.Background()

	instanceID := "a"
	hour := time.Date(2026, 9, 10, 3, 0, 0, 0, time.UTC)
	raw := []domain.MetricSample{
		{InstanceID: &instanceID, At: hour.Add(5 * time.Minute), CPU: 10, Mem: 1000, Players: 1, DiskFree: 5000},
		{InstanceID: &instanceID, At: hour.Add(15 * time.Minute), CPU: 20, Mem: 2000, Players: 3, DiskFree: 4800},
		{InstanceID: &instanceID, At: hour.Add(25 * time.Minute), CPU: 30, Mem: 3000, Players: 2, DiskFree: 4900},
		{InstanceID: &instanceID, At: hour.Add(35 * time.Minute), CPU: 40, Mem: 4000, Players: 4, DiskFree: 4700},
	}
	for _, s := range raw {
		if err := repo.Insert(ctx, s); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	olderThan := hour.Add(1 * time.Hour)
	if err := repo.Downsample(ctx, olderThan); err != nil {
		t.Fatalf("downsample: %v", err)
	}

	got, err := repo.Range(ctx, &instanceID, hour.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("range after downsample: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected the 4 raw rows to collapse into 1 hourly row, got %d: %+v", len(got), got)
	}
	row := got[0]
	if !row.Hourly {
		t.Errorf("expected the collapsed row to be marked hourly, got %+v", row)
	}
	if row.CPU != 25 {
		t.Errorf("expected avg cpu 25, got %v", row.CPU)
	}
	if row.Mem != 2500 {
		t.Errorf("expected avg mem 2500, got %v", row.Mem)
	}
	if row.Players != 4 {
		t.Errorf("expected max players 4, got %v", row.Players)
	}
	if row.DiskFree != 4700 {
		t.Errorf("expected min disk_free 4700, got %v", row.DiskFree)
	}
	if !row.At.Equal(hour) {
		t.Errorf("expected the hourly row's at to be the bucket start %v, got %v", hour, row.At)
	}

	// Idempotent: a second call finds no raw rows left, so nothing changes.
	if err := repo.Downsample(ctx, olderThan); err != nil {
		t.Fatalf("second downsample: %v", err)
	}
	got2, err := repo.Range(ctx, &instanceID, hour.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("range after second downsample: %v", err)
	}
	if len(got2) != 1 || got2[0].CPU != 25 || got2[0].Mem != 2500 {
		t.Fatalf("expected the second downsample to be a no-op, got %+v", got2)
	}
}

func TestMetricSamplesPrune(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	repo := NewMetricSamplesRepo(sqldb)
	ctx := context.Background()

	instanceID := "a"
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	old := domain.MetricSample{InstanceID: &instanceID, At: base, CPU: 1, Mem: 1, Players: 0, DiskFree: 1}
	recent := domain.MetricSample{InstanceID: &instanceID, At: base.Add(200 * 24 * time.Hour), CPU: 2, Mem: 2, Players: 0, DiskFree: 2}
	if err := repo.Insert(ctx, old); err != nil {
		t.Fatalf("insert old: %v", err)
	}
	if err := repo.Insert(ctx, recent); err != nil {
		t.Fatalf("insert recent: %v", err)
	}

	cutoff := base.Add(100 * 24 * time.Hour)
	if err := repo.Prune(ctx, cutoff); err != nil {
		t.Fatalf("prune: %v", err)
	}

	got, err := repo.Range(ctx, &instanceID, base.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("range: %v", err)
	}
	if len(got) != 1 || !got[0].At.Equal(recent.At) {
		t.Fatalf("expected only the recent sample to remain, got %+v", got)
	}
}
