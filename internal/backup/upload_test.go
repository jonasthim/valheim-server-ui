package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func buildZip(t *testing.T, files map[string]string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return &buf
}

func TestUpload_RejectsNonZipExtension(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	_, err := env.svc.Upload(context.Background(), "main", "backup.tar.gz", bytes.NewReader([]byte("whatever")))
	de := requireDomainError(t, err)
	if de.Code != domain.CodeValidationFailed {
		t.Errorf("expected validation_failed, got %v", de.Code)
	}
}

func TestUpload_RejectsZipWithoutManifestOrDB(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	z := buildZip(t, map[string]string{"readme.txt": "hello"})
	_, err := env.svc.Upload(context.Background(), "main", "backup.zip", z)
	de := requireDomainError(t, err)
	if de.Code != domain.CodeValidationFailed {
		t.Errorf("expected validation_failed, got %v", de.Code)
	}
}

func TestUpload_AcceptsZipWithManifest(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	z := buildZip(t, map[string]string{
		"manifest.json":             `{"instance_id":"main","world":"Dedicated","kind":"manual","app_version":"x"}`,
		"worlds_local/Dedicated.db": "db-content",
	})
	b, err := env.svc.Upload(context.Background(), "main", "my-backup.zip", z)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if b.Kind != domain.BackupUploaded || b.World != "Dedicated" {
		t.Errorf("unexpected uploaded backup: %+v", b)
	}

	list, err := env.svc.List(context.Background(), "main")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if _, ok := findBackup(list, b.Filename); !ok {
		t.Errorf("expected uploaded backup to appear in List")
	}
}

func TestUpload_AcceptsZipWithOnlyDB(t *testing.T) {
	env := newTestEnv(t)
	env.createInstance("main", "Dedicated", 2456)

	z := buildZip(t, map[string]string{"worlds_local/OtherWorld.db": "db-content"})
	b, err := env.svc.Upload(context.Background(), "main", "world-only.zip", z)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if b.World != "OtherWorld" {
		t.Errorf("expected world inferred from the .db entry, got %q", b.World)
	}
}
