package players

import (
	"context"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/db"
)

func TestSQLStore_UpsertUpdateNameList(t *testing.T) {
	ctx := context.Background()
	sqldb, err := db.OpenMemory(ctx)
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer sqldb.Close()

	// The players table has a foreign key to instances(id); seed one row so
	// the constraint is satisfied (mirrors what the instance service does).
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := sqldb.ExecContext(ctx, `INSERT INTO instances (id, name, config_json, created_at, updated_at) VALUES (?, ?, '{}', ?, ?)`,
		"main", "Main", now, now); err != nil {
		t.Fatalf("seed instance: %v", err)
	}

	store := NewSQLStore(sqldb)

	t1 := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	if err := store.Upsert(ctx, "main", "76561198000000001", "", t1); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	t2 := t1.Add(2 * time.Second)
	if err := store.UpdateName(ctx, "main", "76561198000000001", "Bjorn", t2); err != nil {
		t.Fatalf("UpdateName: %v", err)
	}

	list, err := store.List(ctx, "main", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List = %#v, want 1 row", list)
	}
	kp := list[0]
	if kp.PlatformID != "76561198000000001" || kp.Name != "Bjorn" || kp.SessionCount != 1 {
		t.Fatalf("row = %#v, want platform_id/Bjorn/session_count=1", kp)
	}
	if !kp.FirstSeenAt.Equal(t1) {
		t.Errorf("FirstSeenAt = %v, want %v", kp.FirstSeenAt, t1)
	}
	if !kp.LastSeenAt.Equal(t2) {
		t.Errorf("LastSeenAt = %v, want %v", kp.LastSeenAt, t2)
	}

	// A second join bumps session_count and last_seen, keeps first_seen.
	t3 := t2.Add(time.Hour)
	if err := store.Upsert(ctx, "main", "76561198000000001", "", t3); err != nil {
		t.Fatalf("Upsert (2nd join): %v", err)
	}
	list, err = store.List(ctx, "main", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	kp = list[0]
	if kp.SessionCount != 2 {
		t.Errorf("SessionCount = %d, want 2", kp.SessionCount)
	}
	if kp.Name != "Bjorn" {
		t.Errorf("Name = %q, want kept as Bjorn (Upsert with empty name must not clobber it)", kp.Name)
	}
	if !kp.FirstSeenAt.Equal(t1) {
		t.Errorf("FirstSeenAt after 2nd join = %v, want unchanged %v", kp.FirstSeenAt, t1)
	}
}

func TestSQLStore_NilDBIsNoOp(t *testing.T) {
	store := NewSQLStore(nil)
	ctx := context.Background()
	if err := store.Upsert(ctx, "main", "id", "name", time.Now()); err != nil {
		t.Errorf("Upsert with nil db: %v", err)
	}
	if err := store.UpdateName(ctx, "main", "id", "name", time.Now()); err != nil {
		t.Errorf("UpdateName with nil db: %v", err)
	}
	list, err := store.List(ctx, "main", 10)
	if err != nil || list != nil {
		t.Errorf("List with nil db = %v, %v, want nil, nil", list, err)
	}
}
