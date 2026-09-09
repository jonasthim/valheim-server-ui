package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// seedBackup inserts a backup row and a matching (empty) file on disk, dated
// ageHours in the past, so retention has something real to remove.
func (e *testEnv) seedBackup(instanceID string, kind domain.BackupKind, ageHours float64) backupRow {
	e.t.Helper()
	ctx := context.Background()
	createdAt := time.Now().UTC().Add(-time.Duration(ageHours * float64(time.Hour)))
	filename := backupFilename(instanceID, "Dedicated", createdAt, kind)
	paths := e.inst.Paths(instanceID)
	if err := os.MkdirAll(paths.Backups, 0o750); err != nil {
		e.t.Fatalf("mkdir backups dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.Backups, filename), []byte("zip"), 0o640); err != nil {
		e.t.Fatalf("write backup file: %v", err)
	}
	row := backupRow{InstanceID: instanceID, World: "Dedicated", Kind: kind, Filename: filename, SizeBytes: 3, CreatedAt: createdAt}
	id, err := e.svc.insertBackupRow(ctx, row)
	if err != nil {
		e.t.Fatalf("insert backup row: %v", err)
	}
	row.ID = id
	return row
}

func (e *testEnv) backupExists(instanceID string, id int64) bool {
	e.t.Helper()
	_, err := e.svc.getBackupRow(context.Background(), instanceID, id)
	return err == nil
}

func TestRetention_KeepLastOnly(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	var rows []backupRow
	for i := 0; i < 5; i++ {
		rows = append(rows, env.seedBackup("main", domain.BackupScheduled, float64(i))) // 0h,1h,2h,3h,4h old
	}

	cfg := domain.InstanceConfig{BackupKeepLast: 2, BackupKeepDays: 0}
	if err := env.svc.applyRetention(context.Background(), "main", cfg); err != nil {
		t.Fatalf("applyRetention: %v", err)
	}

	// Newest 2 (rows[0], rows[1]) survive; the rest are gone.
	for i, r := range rows {
		want := i < 2
		if got := env.backupExists("main", r.ID); got != want {
			t.Errorf("row %d (age %dh): exists=%v, want %v", i, i, got, want)
		}
	}
}

func TestRetention_KeepDaysOnly(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	fresh := env.seedBackup("main", domain.BackupScheduled, 12)  // 12h old
	old := env.seedBackup("main", domain.BackupPreUpdate, 48)    // 2 days old
	older := env.seedBackup("main", domain.BackupPreRestore, 72) // 3 days old

	cfg := domain.InstanceConfig{BackupKeepLast: 0, BackupKeepDays: 1}
	if err := env.svc.applyRetention(context.Background(), "main", cfg); err != nil {
		t.Fatalf("applyRetention: %v", err)
	}

	if !env.backupExists("main", fresh.ID) {
		t.Errorf("expected the 12h-old backup to survive")
	}
	if env.backupExists("main", old.ID) {
		t.Errorf("expected the 2-day-old backup to be deleted")
	}
	if env.backupExists("main", older.ID) {
		t.Errorf("expected the 3-day-old backup to be deleted")
	}
}

func TestRetention_BothLimits_Intersection(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	// Newest 2 by count, but one of those two is already older than the day
	// limit -- it must still be deleted (surviving requires satisfying BOTH
	// limits, not either).
	newest := env.seedBackup("main", domain.BackupScheduled, 1)           // 1h old, rank 0
	secondOldEnough := env.seedBackup("main", domain.BackupScheduled, 30) // 30h old, rank 1 but > 1 day
	third := env.seedBackup("main", domain.BackupScheduled, 40)           // rank 2, outside keep_last anyway

	cfg := domain.InstanceConfig{BackupKeepLast: 2, BackupKeepDays: 1}
	if err := env.svc.applyRetention(context.Background(), "main", cfg); err != nil {
		t.Fatalf("applyRetention: %v", err)
	}

	if !env.backupExists("main", newest.ID) {
		t.Errorf("expected the newest backup to survive")
	}
	if env.backupExists("main", secondOldEnough.ID) {
		t.Errorf("expected the second-newest (but >1 day old) backup to be deleted")
	}
	if env.backupExists("main", third.ID) {
		t.Errorf("expected the third-newest backup to be deleted")
	}
}

func TestRetention_UnlimitedWhenBothZero(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	rows := []backupRow{
		env.seedBackup("main", domain.BackupScheduled, 0),
		env.seedBackup("main", domain.BackupScheduled, 1000), // very old
	}

	cfg := domain.InstanceConfig{BackupKeepLast: 0, BackupKeepDays: 0}
	if err := env.svc.applyRetention(context.Background(), "main", cfg); err != nil {
		t.Fatalf("applyRetention: %v", err)
	}
	for _, r := range rows {
		if !env.backupExists("main", r.ID) {
			t.Errorf("expected backup %d to survive when both limits are 0 (unlimited)", r.ID)
		}
	}
}

func TestRetention_NeverTouchesManualOrUploaded(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	manual := env.seedBackup("main", domain.BackupManual, 1000)
	uploaded := env.seedBackup("main", domain.BackupUploaded, 1000)
	// A tight retention that would delete everything auto-deletable.
	autoOld := env.seedBackup("main", domain.BackupScheduled, 1000)

	cfg := domain.InstanceConfig{BackupKeepLast: 0, BackupKeepDays: 1}
	if err := env.svc.applyRetention(context.Background(), "main", cfg); err != nil {
		t.Fatalf("applyRetention: %v", err)
	}

	if !env.backupExists("main", manual.ID) {
		t.Errorf("manual backups must never be auto-deleted")
	}
	if !env.backupExists("main", uploaded.ID) {
		t.Errorf("uploaded backups must never be auto-deleted")
	}
	if env.backupExists("main", autoOld.ID) {
		t.Errorf("expected the old scheduled backup to be deleted")
	}
}

// TestCreate_TriggersRetention checks that Create itself runs retention for
// non-manual kinds, and skips it for manual ones.
func TestCreate_TriggersRetention(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldFiles("main", "Dedicated", []byte("db"), []byte("fwl"))

	// Update the instance's retention policy to a tight keep_last=1.
	cfg := domain.InstanceConfig{Name: "Test main", World: "Dedicated", Password: "secret123", Port: 2456, Public: true, BackupKeepLast: 1, BackupKeepDays: 0}
	cfg.ApplyDefaults()
	if _, err := env.inst.Update(context.Background(), "main", nil, &cfg, nil); err != nil {
		t.Fatalf("Update instance config: %v", err)
	}

	old := env.seedBackup("main", domain.BackupScheduled, 5)

	if _, err := env.svc.Create(context.Background(), "main", domain.BackupScheduled, ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if env.backupExists("main", old.ID) {
		t.Errorf("expected retention (keep_last=1) to remove the older scheduled backup")
	}

	// A manual Create must not trigger retention.
	old2 := env.seedBackup("main", domain.BackupScheduled, 5)
	if _, err := env.svc.Create(context.Background(), "main", domain.BackupManual, ""); err != nil {
		t.Fatalf("Create manual: %v", err)
	}
	if !env.backupExists("main", old2.ID) {
		t.Errorf("a manual backup must not trigger retention on other backups")
	}
}
