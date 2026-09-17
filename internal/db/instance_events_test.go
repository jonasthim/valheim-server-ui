package db

import (
	"context"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestInstanceEventsInsertListCountLatest(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	repo := NewInstanceEventRepo(sqldb)
	ctx := context.Background()

	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	events := []domain.InstanceEvent{
		{InstanceID: "main", At: base, Kind: "start"},
		{InstanceID: "main", At: base.Add(1 * time.Minute), Kind: "crash", Detail: "exit status 1"},
		{InstanceID: "main", At: base.Add(2 * time.Minute), Kind: "crash", Detail: "exit status 139"},
	}
	for _, e := range events {
		if err := repo.Insert(ctx, e); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	// A different instance's events must not leak into main's results.
	if err := repo.Insert(ctx, domain.InstanceEvent{InstanceID: "other", At: base, Kind: "start"}); err != nil {
		t.Fatalf("insert other: %v", err)
	}

	// List: newest first.
	list, err := repo.List(ctx, "main", 100, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 events, got %d", len(list))
	}
	if list[0].Kind != "crash" || list[0].Detail != "exit status 139" {
		t.Errorf("expected the newest (2nd crash) first, got %+v", list[0])
	}
	if list[2].Kind != "start" {
		t.Errorf("expected the oldest (start) last, got %+v", list[2])
	}

	// List: limit.
	limited, err := repo.List(ctx, "main", 1, nil)
	if err != nil || len(limited) != 1 {
		t.Fatalf("expected limit=1, got %d err=%v", len(limited), err)
	}
	if limited[0].Detail != "exit status 139" {
		t.Errorf("expected the newest event, got %+v", limited[0])
	}

	// List: before cursor excludes events at/after the cutoff.
	before := base.Add(2 * time.Minute)
	rest, err := repo.List(ctx, "main", 100, &before)
	if err != nil {
		t.Fatalf("list before: %v", err)
	}
	if len(rest) != 2 {
		t.Fatalf("expected 2 events strictly before the cutoff, got %d", len(rest))
	}

	// CountSince counts only the given kind at/after the cutoff.
	cutoff := base.Add(90 * time.Second) // between the two crash events
	count, err := repo.CountSince(ctx, "main", "crash", cutoff)
	if err != nil {
		t.Fatalf("count since: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 crash since cutoff, got %d", count)
	}
	allCrashes, err := repo.CountSince(ctx, "main", "crash", base)
	if err != nil {
		t.Fatalf("count since base: %v", err)
	}
	if allCrashes != 2 {
		t.Errorf("expected 2 crashes since base, got %d", allCrashes)
	}
	startCount, err := repo.CountSince(ctx, "main", "start", base)
	if err != nil {
		t.Fatalf("count since (start): %v", err)
	}
	if startCount != 1 {
		t.Errorf("expected 1 start event, got %d", startCount)
	}

	// Latest returns the newest event of the given kind, or nil.
	latest, err := repo.Latest(ctx, "main", "crash")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest == nil || latest.Detail != "exit status 139" {
		t.Fatalf("expected the latest crash, got %+v", latest)
	}

	none, err := repo.Latest(ctx, "missing-instance", "crash")
	if err != nil {
		t.Fatalf("latest for missing instance: %v", err)
	}
	if none != nil {
		t.Errorf("expected nil for an instance with no crash events, got %+v", none)
	}
}
