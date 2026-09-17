package db

import (
	"context"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestNotificationLogInsertAndList(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	repo := NewNotificationLogRepo(sqldb)
	ctx := context.Background()

	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	entries := []domain.NotificationLogEntry{
		{At: base, ChannelID: "c1", Kind: domain.AlertCrashed, InstanceID: "main", OK: true},
		{At: base.Add(1 * time.Minute), ChannelID: "c1", Kind: domain.AlertJobFailed, InstanceID: "main", OK: false, Error: "HTTP 500"},
		{At: base.Add(2 * time.Minute), ChannelID: "c2", Kind: domain.AlertDiskLow, OK: true},
	}
	for _, e := range entries {
		if err := repo.Insert(ctx, e); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	// List: newest first.
	list, err := repo.List(ctx, 100, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(list))
	}
	if list[0].Kind != domain.AlertDiskLow || list[0].ChannelID != "c2" {
		t.Errorf("expected the newest (disk_low) first, got %+v", list[0])
	}
	if list[0].ID == 0 {
		t.Errorf("expected a non-zero id, got %+v", list[0])
	}
	if list[2].Kind != domain.AlertCrashed {
		t.Errorf("expected the oldest (crashed) last, got %+v", list[2])
	}
	if !list[2].OK {
		t.Errorf("expected the crashed entry to be ok=true, got %+v", list[2])
	}
	if list[1].OK || list[1].Error != "HTTP 500" {
		t.Errorf("expected the failed entry to carry its error, got %+v", list[1])
	}

	// List: limit.
	limited, err := repo.List(ctx, 1, nil)
	if err != nil || len(limited) != 1 {
		t.Fatalf("expected limit=1, got %d err=%v", len(limited), err)
	}
	if limited[0].Kind != domain.AlertDiskLow {
		t.Errorf("expected the newest entry, got %+v", limited[0])
	}

	// List: before cursor excludes entries at/after the cutoff.
	before := base.Add(2 * time.Minute)
	rest, err := repo.List(ctx, 100, &before)
	if err != nil {
		t.Fatalf("list before: %v", err)
	}
	if len(rest) != 2 {
		t.Fatalf("expected 2 entries strictly before the cutoff, got %d", len(rest))
	}
}

func TestNotificationLogList_EmptyWhenNoRows(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	repo := NewNotificationLogRepo(sqldb)

	list, err := repo.List(context.Background(), 100, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no entries, got %+v", list)
	}
}
