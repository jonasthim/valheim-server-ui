package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func findBackup(list []domain.Backup, filename string) (domain.Backup, bool) {
	for _, b := range list {
		if b.Filename == filename {
			return b, true
		}
	}
	return domain.Backup{}, false
}

func TestList_ReconcilesMissingFile(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)
	env.writeWorldFiles("main", "Dedicated", []byte("db"), []byte("fwl"))

	b, err := env.svc.Create(context.Background(), "main", domain.BackupManual, "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	full := filepath.Join(env.inst.Paths("main").Backups, b.Filename)
	if err := os.Remove(full); err != nil {
		t.Fatalf("remove backup file: %v", err)
	}

	list, err := env.svc.List(context.Background(), "main")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got, ok := findBackup(list, b.Filename)
	if !ok {
		t.Fatalf("expected row for %s to still be listed", b.Filename)
	}
	if !got.Missing {
		t.Errorf("expected Missing=true for a row whose file was removed")
	}
}

func TestList_ReconcilesStrayFile_RecognisedName(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	paths := env.inst.Paths("main")
	if err := os.MkdirAll(paths.Backups, 0o750); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}
	// Matches the naming scheme but was never inserted as a row (e.g.
	// hand-copied from another host).
	ts, err := time.Parse(time.RFC3339, "2026-01-02T03:04:05Z")
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	filename := backupFilename("main", "Dedicated", ts, domain.BackupScheduled)
	if err := os.WriteFile(filepath.Join(paths.Backups, filename), []byte("zip"), 0o640); err != nil {
		t.Fatalf("write stray file: %v", err)
	}

	list, err := env.svc.List(context.Background(), "main")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got, ok := findBackup(list, filename)
	if !ok {
		t.Fatalf("expected stray file %s to be reconciled into the list", filename)
	}
	if got.Missing {
		t.Errorf("a present file must not be reported Missing")
	}
	if got.World != "Dedicated" || got.Kind != domain.BackupScheduled {
		t.Errorf("expected the stray file's kind/world to be recovered from its name, got %+v", got)
	}
	if got.ID == 0 {
		t.Errorf("expected the reconciled row to have been inserted with an id")
	}

	// A second List call must not duplicate the row.
	list2, err := env.svc.List(context.Background(), "main")
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	count := 0
	for _, b := range list2 {
		if b.Filename == filename {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one row for %s after reconciling twice, got %d", filename, count)
	}
}

func TestList_ReconcilesStrayFile_UnrecognisedName(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	paths := env.inst.Paths("main")
	if err := os.MkdirAll(paths.Backups, 0o750); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}
	filename := "some-old-backup.zip"
	if err := os.WriteFile(filepath.Join(paths.Backups, filename), []byte("zip"), 0o640); err != nil {
		t.Fatalf("write stray file: %v", err)
	}

	list, err := env.svc.List(context.Background(), "main")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got, ok := findBackup(list, filename)
	if !ok {
		t.Fatalf("expected unrecognised stray file %s to still be listed", filename)
	}
	if got.Kind != domain.BackupManual || got.World != "unknown" {
		t.Errorf("expected fallback kind=manual world=unknown, got %+v", got)
	}
}
