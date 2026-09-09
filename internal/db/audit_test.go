package db

import (
	"context"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestAuditInsertAndList(t *testing.T) {
	sqldb, err := OpenMemory(context.Background())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()
	repo := NewAuditRepo(sqldb)
	ctx := context.Background()

	uid := int64(1)
	entries := []domain.AuditEntry{
		{UserID: &uid, Username: "admin", Action: "auth.login", InstanceID: "", Target: "admin", IP: "1.1.1.1"},
		{UserID: &uid, Username: "admin", Action: "instance.start", InstanceID: "main", Target: "main", IP: "1.1.1.1"},
		{Username: "op", Action: "auth.login", InstanceID: "", Target: "op", IP: "2.2.2.2", Details: map[string]any{"k": "v"}},
	}
	for _, e := range entries {
		if err := repo.Insert(ctx, e); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	all, err := repo.List(ctx, "", "", 100, 0)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(all))
	}
	// newest first
	if all[0].Action != "auth.login" || all[0].Username != "op" {
		t.Fatalf("expected newest first, got %+v", all[0])
	}
	if all[0].Details["k"] != "v" {
		t.Fatalf("details not round-tripped: %+v", all[0].Details)
	}

	byInstance, err := repo.List(ctx, "main", "", 100, 0)
	if err != nil || len(byInstance) != 1 {
		t.Fatalf("expected 1 entry for instance main, got %d err=%v", len(byInstance), err)
	}

	byUser, err := repo.List(ctx, "", "admin", 100, 0)
	if err != nil || len(byUser) != 2 {
		t.Fatalf("expected 2 entries for admin, got %d err=%v", len(byUser), err)
	}

	limited, err := repo.List(ctx, "", "", 1, 0)
	if err != nil || len(limited) != 1 {
		t.Fatalf("expected limit=1, got %d err=%v", len(limited), err)
	}
	before := limited[0].ID
	rest, err := repo.List(ctx, "", "", 100, before)
	if err != nil || len(rest) != 2 {
		t.Fatalf("expected 2 remaining entries with before cursor, got %d err=%v", len(rest), err)
	}
}
