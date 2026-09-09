package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestRemoveWorldFilesAlsoDropsValheimCopies(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"Midgard.db", "Midgard.fwl", "Midgard.db.old", "Midgard_backup_auto-20260909144803.fwl", "Midgard_backup_auto-20260909144803.db", "Other.db", "Other.fwl", "Midgardia.db"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := removeWorldFiles(dir, "Midgard")
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	left, _ := os.ReadDir(dir)
	var names []string
	for _, e := range left {
		names = append(names, e.Name())
	}
	if len(names) != 3 {
		t.Fatalf("expected Other.db, Other.fwl and Midgardia.db to survive, got %v", names)
	}
	removed, err = removeWorldFiles(dir, "Nope")
	if err != nil || removed {
		t.Fatalf("nothing should be removed for an unknown world: removed=%v err=%v", removed, err)
	}
}

func TestEnqueueWorldRegenerateRules(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Midgard", 2456)
	ctx := context.Background()
	if _, err := env.svc.EnqueueWorldRegenerate(ctx, "main", "Other", false, "tester"); err == nil || domain.AsError(err).Code != domain.CodeConflict {
		t.Fatalf("regenerating a non-active world must be a conflict, got %v", err)
	}
	if _, err := env.svc.EnqueueWorldRegenerate(ctx, "main", "../evil", false, "tester"); err == nil || domain.AsError(err).Code != domain.CodeValidationFailed {
		t.Fatalf("invalid name must fail validation, got %v", err)
	}
}

func TestWorldRegenerateJobBacksUpAndDeletes(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Midgard", 2456)
	env.writeWorldFiles("main", "Midgard", []byte("db-data"), []byte("fwl-data"))
	dir := env.inst.Paths("main").WorldsDir()
	if err := os.WriteFile(filepath.Join(dir, "Midgard_backup_auto-20260909144803.fwl"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	job, err := env.svc.EnqueueWorldRegenerate(ctx, "main", "Midgard", false, "tester")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	done, err := env.run.WaitFor(ctx, job.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if done.Status != domain.JobSucceeded {
		t.Fatalf("job %s: %s", done.Status, done.Error)
	}
	for _, n := range []string{"Midgard.db", "Midgard.fwl", "Midgard_backup_auto-20260909144803.fwl"} {
		if _, err := os.Stat(filepath.Join(dir, n)); !os.IsNotExist(err) {
			t.Errorf("%s should be gone", n)
		}
	}
	backups, err := env.svc.List(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 || backups[0].Kind != domain.BackupManual || backups[0].World != "Midgard" {
		t.Fatalf("expected one manual safety backup of Midgard, got %+v", backups)
	}
}

func TestWorldRegenerateJob_DirectoryLayout_BacksUpAndDeletes(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Midgard", 2456)
	dir := env.writeWorldDir("main", "Midgard", worldGen{N: 7, Committed: true})
	rolling := env.writeWorldDir("main", "Midgard_backup_auto-20260909144803", worldGen{N: 6, Committed: true})

	ctx := context.Background()
	job, err := env.svc.EnqueueWorldRegenerate(ctx, "main", "Midgard", false, "tester")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	done, err := env.run.WaitFor(ctx, job.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if done.Status != domain.JobSucceeded {
		t.Fatalf("job %s: %s", done.Status, done.Error)
	}
	for _, p := range []string{dir, rolling} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should be gone", p)
		}
	}
	backups, err := env.svc.List(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 || backups[0].Kind != domain.BackupManual {
		t.Fatalf("expected one manual safety backup before deleting a directory world, got %+v", backups)
	}
}
