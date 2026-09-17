package remote

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func fakeRclonePath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("testdata", "fake-rclone.sh"))
	if err != nil {
		t.Fatalf("resolve fake rclone path: %v", err)
	}
	return p
}

// writeZip creates a small file at dir/name and returns its full path,
// standing in for a real backup zip (uploaders never inspect its content).
func writeZip(t *testing.T, dir, name, content string) string {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte(content), 0o640); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	return full
}

// ---------------------------------------------------------------- local

func TestLocalUpload_CopiesToInstanceSubdir(t *testing.T) {
	src := t.TempDir()
	zip := writeZip(t, src, "main-Dedicated-20260101T000000Z-manual.zip", "zip-bytes")

	destRoot := t.TempDir()
	target := domain.BackupTarget{Type: domain.BackupTargetLocal, Path: destRoot}

	d := New("")
	if err := d.Upload(context.Background(), target, zip, "main"); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	want := filepath.Join(destRoot, "main", "main-Dedicated-20260101T000000Z-manual.zip")
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("expected copied file at %s: %v", want, err)
	}
	if string(got) != "zip-bytes" {
		t.Errorf("copied content = %q, want %q", got, "zip-bytes")
	}
}

func TestLocalUpload_KeepLastPrunesOldest(t *testing.T) {
	src := t.TempDir()
	destRoot := t.TempDir()
	target := domain.BackupTarget{Type: domain.BackupTargetLocal, Path: destRoot, KeepLast: 2}
	d := New("")
	ctx := context.Background()

	names := []string{"a.zip", "b.zip", "c.zip"}
	for _, n := range names {
		zip := writeZip(t, src, n, "content-"+n)
		if err := d.Upload(ctx, target, zip, "main"); err != nil {
			t.Fatalf("Upload %s: %v", n, err)
		}
	}

	entries, err := os.ReadDir(filepath.Join(destRoot, "main"))
	if err != nil {
		t.Fatalf("read dest dir: %v", err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	sort.Strings(got)
	want := []string{"b.zip", "c.zip"}
	if len(got) != len(want) {
		t.Fatalf("expected only the 2 newest files to remain, got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("remaining files = %v, want %v", got, want)
			break
		}
	}
}

// ---------------------------------------------------------------- rclone

func TestRcloneUpload_InvokesCopytoWithExpectedArgs(t *testing.T) {
	src := t.TempDir()
	zip := writeZip(t, src, "main-Dedicated-20260101T000000Z-manual.zip", "zip-bytes")
	logFile := filepath.Join(t.TempDir(), "argv.log")
	t.Setenv("FAKE_RCLONE_LOG", logFile)

	target := domain.BackupTarget{Type: domain.BackupTargetRclone, Remote: "b2:my-bucket/valheim"}
	d := New(fakeRclonePath(t))
	if err := d.Upload(context.Background(), target, zip, "main"); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	got, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read argv log: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(got), "\n"), "\n")
	want := []string{"copyto", zip, "b2:my-bucket/valheim/main/main-Dedicated-20260101T000000Z-manual.zip"}
	if len(lines) != len(want) {
		t.Fatalf("argv = %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestRcloneUpload_FailureIncludesOutput(t *testing.T) {
	src := t.TempDir()
	zip := writeZip(t, src, "main-Dedicated-20260101T000000Z-manual.zip", "zip-bytes")
	t.Setenv("FAKE_RCLONE_FAIL", "1")

	target := domain.BackupTarget{Type: domain.BackupTargetRclone, Remote: "b2:my-bucket/valheim"}
	d := New(fakeRclonePath(t))
	err := d.Upload(context.Background(), target, zip, "main")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected error to contain %q, got %q", "boom", err.Error())
	}
}

func TestRcloneUpload_NotInstalled(t *testing.T) {
	src := t.TempDir()
	zip := writeZip(t, src, "x.zip", "zip-bytes")

	target := domain.BackupTarget{Type: domain.BackupTargetRclone, Remote: "b2:my-bucket/valheim"}
	d := New("") // rclone not found on host
	err := d.Upload(context.Background(), target, zip, "main")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "rclone is not installed") {
		t.Errorf("expected a not-installed error, got %q", err.Error())
	}
}

// ---------------------------------------------------------------- dispatch

func TestUpload_UnknownTargetType(t *testing.T) {
	d := New("")
	err := d.Upload(context.Background(), domain.BackupTarget{Type: "carrier-pigeon"}, "/tmp/x.zip", "main")
	if err == nil {
		t.Fatal("expected an error for an unknown target type")
	}
}
