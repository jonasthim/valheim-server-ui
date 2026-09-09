package instance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestRotateConsoleLog_NoRotationBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	paths := domain.PathsFor(dir, "main")
	if err := os.MkdirAll(paths.Logs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ConsoleLog(), []byte("small"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rotateConsoleLog(paths); err != nil {
		t.Fatalf("rotateConsoleLog: %v", err)
	}
	if _, err := os.Stat(paths.ConsoleLog()); err != nil {
		t.Errorf("console.log should not have been rotated: %v", err)
	}
}

func TestRotateConsoleLog_MissingFile(t *testing.T) {
	dir := t.TempDir()
	paths := domain.PathsFor(dir, "main")
	if err := os.MkdirAll(paths.Logs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := rotateConsoleLog(paths); err != nil {
		t.Errorf("rotating a missing console.log should be a no-op, got %v", err)
	}
}

func TestRotateConsoleLog_RotatesAndPrunes(t *testing.T) {
	dir := t.TempDir()
	paths := domain.PathsFor(dir, "main")
	if err := os.MkdirAll(paths.Logs, 0o755); err != nil {
		t.Fatal(err)
	}
	big := make([]byte, maxConsoleLogSize+1)
	if err := os.WriteFile(paths.ConsoleLog(), big, 0o644); err != nil {
		t.Fatal(err)
	}
	// Pre-seed more rotated files than we keep, with names that sort before
	// the new one, to exercise pruning.
	for i := 0; i < keepRotatedLogs+2; i++ {
		name := filepath.Join(paths.Logs, "console-2020010"+string(rune('0'+i))+"T000000Z.log")
		if err := os.WriteFile(name, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := rotateConsoleLog(paths); err != nil {
		t.Fatalf("rotateConsoleLog: %v", err)
	}
	if _, err := os.Stat(paths.ConsoleLog()); !os.IsNotExist(err) {
		t.Errorf("expected console.log to be renamed away, stat err = %v", err)
	}

	entries, err := os.ReadDir(paths.Logs)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".log" {
			count++
		}
	}
	if count != keepRotatedLogs {
		t.Errorf("expected exactly %d rotated logs to survive pruning, got %d", keepRotatedLogs, count)
	}
}

func TestRemoveTree_RefusesOutsideInstancesDir(t *testing.T) {
	instancesDir := t.TempDir()
	outside := t.TempDir()
	if err := removeTree(instancesDir, outside); err == nil {
		t.Fatal("expected removeTree to refuse a path outside instancesDir")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("outside directory should be untouched: %v", err)
	}
}

func TestRemoveTree_RefusesInstancesDirItself(t *testing.T) {
	instancesDir := t.TempDir()
	if err := removeTree(instancesDir, instancesDir); err == nil {
		t.Fatal("expected removeTree to refuse deleting instancesDir itself")
	}
}

func TestRemoveTree_RemovesInstanceDir(t *testing.T) {
	instancesDir := t.TempDir()
	target := filepath.Join(instancesDir, "main")
	if err := os.MkdirAll(filepath.Join(target, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := removeTree(instancesDir, target); err != nil {
		t.Fatalf("removeTree: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed, stat err = %v", target, err)
	}
}
