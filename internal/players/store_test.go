package players

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// newSeededStore opens an in-memory DB with migrations applied and seeds one
// instance row (the players/player_sessions tables reference instances(id)),
// mirroring the seeding TestSQLStore_UpsertUpdateNameList does inline.
func newSeededStore(t *testing.T, instanceID string) (*SQLStore, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	sqldb, err := db.OpenMemory(ctx)
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := sqldb.ExecContext(ctx, `INSERT INTO instances (id, name, config_json, created_at, updated_at) VALUES (?, ?, '{}', ?, ?)`,
		instanceID, instanceID, now, now); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	return NewSQLStore(sqldb), sqldb
}

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
	if err := store.OpenSession(ctx, "main", "id", time.Now()); err != nil {
		t.Errorf("OpenSession with nil db: %v", err)
	}
	if dur, err := store.CloseSession(ctx, "main", "id", time.Now()); err != nil || dur != 0 {
		t.Errorf("CloseSession with nil db = %v, %v, want 0, nil", dur, err)
	}
	if err := store.CloseAll(ctx, "main", time.Now()); err != nil {
		t.Errorf("CloseAll with nil db: %v", err)
	}
	if err := store.SetNote(ctx, "main", "id", "note"); err != nil {
		t.Errorf("SetNote with nil db: %v", err)
	}
}

func TestSQLStore_OpenCloseSessionTracksPlaytime(t *testing.T) {
	ctx := context.Background()
	store, _ := newSeededStore(t, "main")

	start := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	if err := store.Upsert(ctx, "main", "id-1", "Bjorn", start); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := store.OpenSession(ctx, "main", "id-1", start); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	end := start.Add(90 * time.Second)
	dur, err := store.CloseSession(ctx, "main", "id-1", end)
	if err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	if dur != 90*time.Second {
		t.Errorf("CloseSession duration = %v, want 90s", dur)
	}

	list, err := store.List(ctx, "main", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List = %#v, want 1 row", list)
	}
	if list[0].TotalPlaySeconds != 90 || list[0].LastSessionSeconds != 90 {
		t.Errorf("playtime = total=%d last=%d, want 90/90", list[0].TotalPlaySeconds, list[0].LastSessionSeconds)
	}

	// A second, shorter session adds to the total but replaces last_session.
	start2 := end.Add(time.Minute)
	if err := store.OpenSession(ctx, "main", "id-1", start2); err != nil {
		t.Fatalf("OpenSession (2nd): %v", err)
	}
	if _, err := store.CloseSession(ctx, "main", "id-1", start2.Add(10*time.Second)); err != nil {
		t.Fatalf("CloseSession (2nd): %v", err)
	}
	list, err = store.List(ctx, "main", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list[0].TotalPlaySeconds != 100 || list[0].LastSessionSeconds != 10 {
		t.Errorf("playtime after 2nd session = total=%d last=%d, want 100/10", list[0].TotalPlaySeconds, list[0].LastSessionSeconds)
	}
}

func TestSQLStore_CloseSessionWithNoOpenSessionIsZero(t *testing.T) {
	ctx := context.Background()
	store, _ := newSeededStore(t, "main")
	if err := store.Upsert(ctx, "main", "id-1", "Bjorn", time.Now()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	dur, err := store.CloseSession(ctx, "main", "id-1", time.Now())
	if err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	if dur != 0 {
		t.Errorf("CloseSession duration = %v, want 0 (nothing open)", dur)
	}
}

func TestSQLStore_OpenSessionTwiceLeavesOneOpen(t *testing.T) {
	ctx := context.Background()
	store, sqldb := newSeededStore(t, "main")
	if err := store.Upsert(ctx, "main", "id-1", "Bjorn", time.Now()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	start := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	if err := store.OpenSession(ctx, "main", "id-1", start); err != nil {
		t.Fatalf("OpenSession (1st): %v", err)
	}
	if err := store.OpenSession(ctx, "main", "id-1", start.Add(time.Minute)); err != nil {
		t.Fatalf("OpenSession (2nd): %v", err)
	}

	var openCount int
	if err := sqldb.QueryRowContext(ctx, `SELECT COUNT(*) FROM player_sessions WHERE instance_id = ? AND platform_id = ? AND ended_at IS NULL`,
		"main", "id-1").Scan(&openCount); err != nil {
		t.Fatalf("count open sessions: %v", err)
	}
	if openCount != 1 {
		t.Errorf("open sessions = %d, want 1 (opening again must close the stale one first)", openCount)
	}
}

func TestSQLStore_CloseAllClosesOpenSessionsAndAddsDuration(t *testing.T) {
	ctx := context.Background()
	store, _ := newSeededStore(t, "main")
	start := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	if err := store.Upsert(ctx, "main", "id-1", "Bjorn", start); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := store.OpenSession(ctx, "main", "id-1", start); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	end := start.Add(30 * time.Second)
	if err := store.CloseAll(ctx, "main", end); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}

	list, err := store.List(ctx, "main", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].TotalPlaySeconds != 30 || list[0].LastSessionSeconds != 30 {
		t.Fatalf("after CloseAll = %#v, want total/last = 30", list)
	}

	// Nothing left open: a second CloseAll is a safe no-op.
	if err := store.CloseAll(ctx, "main", end.Add(time.Minute)); err != nil {
		t.Fatalf("CloseAll (2nd, no-op): %v", err)
	}
	list, err = store.List(ctx, "main", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list[0].TotalPlaySeconds != 30 {
		t.Errorf("TotalPlaySeconds after no-op CloseAll = %d, want unchanged 30", list[0].TotalPlaySeconds)
	}
}

func TestSQLStore_SetNoteUnknownPlayerNotFound(t *testing.T) {
	ctx := context.Background()
	store, _ := newSeededStore(t, "main")

	err := store.SetNote(ctx, "main", "does-not-exist", "hello")
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeNotFound {
		t.Fatalf("SetNote unknown player err = %v, want domain.NotFound", err)
	}
}

func TestSQLStore_SetNoteAndListRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, _ := newSeededStore(t, "main")
	if err := store.Upsert(ctx, "main", "id-1", "Bjorn", time.Now()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	list, err := store.List(ctx, "main", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].TotalPlaySeconds != 0 || list[0].LastSessionSeconds != 0 || list[0].Note != "" {
		t.Fatalf("fresh player = %#v, want zero playtime and empty note", list[0])
	}

	if err := store.SetNote(ctx, "main", "id-1", "friendly builder"); err != nil {
		t.Fatalf("SetNote: %v", err)
	}
	list, err = store.List(ctx, "main", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list[0].Note != "friendly builder" {
		t.Errorf("Note = %q, want %q", list[0].Note, "friendly builder")
	}
}
