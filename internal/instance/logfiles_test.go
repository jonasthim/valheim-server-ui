package instance

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// buildLogTree creates console.log, two rotated console-<ts>.log files (with
// mtimes set opposite to their name order, so a test asserting "sorted by
// ModifiedAt" cannot pass by accident of lexical name sort) and a BepInEx
// LogOutput.log, under a fresh instance directory tree.
func buildLogTree(t *testing.T) domain.InstancePaths {
	t.Helper()
	dir := t.TempDir()
	paths := domain.PathsFor(dir, "main")
	if err := os.MkdirAll(paths.Logs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.BepInExDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, paths.ConsoleLog(), "live line 1\nlive line 2\n")

	older := filepath.Join(paths.Logs, "console-20260101T000000Z.log")
	newer := filepath.Join(paths.Logs, "console-20260201T000000Z.log")
	// Names sort "older" < "newer" lexically, but mtimes are reversed: the
	// lexically-newer name gets the OLDER mtime, so a test only passes if
	// listLogFiles actually orders by ModifiedAt, not by name.
	writeFile(t, older, "rotated old-name line\n")
	writeFile(t, newer, "rotated new-name line\n")
	now := time.Now().UTC()
	if err := os.Chtimes(older, now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(paths.BepInExDir(), "LogOutput.log"), "bepinex line\n")

	return paths
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListLogFiles_OrderAndKinds(t *testing.T) {
	paths := buildLogTree(t)

	files, err := listLogFiles(paths)
	if err != nil {
		t.Fatalf("listLogFiles: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("expected 4 log files, got %d: %+v", len(files), files)
	}

	if files[0].Info.Name != "console.log" || files[0].Info.Kind != "console" {
		t.Errorf("files[0] = %+v, want console.log/console", files[0].Info)
	}
	if files[0].Path != paths.ConsoleLog() {
		t.Errorf("files[0].Path = %q, want %q", files[0].Path, paths.ConsoleLog())
	}

	// Rotated files: newest ModifiedAt first, regardless of name order.
	if files[1].Info.Name != "console-20260101T000000Z.log" || files[1].Info.Kind != "rotated" {
		t.Errorf("files[1] = %+v, want the mtime-newer rotated file first", files[1].Info)
	}
	if files[2].Info.Name != "console-20260201T000000Z.log" || files[2].Info.Kind != "rotated" {
		t.Errorf("files[2] = %+v, want the mtime-older rotated file second", files[2].Info)
	}
	if !files[1].Info.ModifiedAt.After(files[2].Info.ModifiedAt) {
		t.Errorf("rotated files not sorted newest-first: %v vs %v", files[1].Info.ModifiedAt, files[2].Info.ModifiedAt)
	}

	if files[3].Info.Name != "LogOutput.log" || files[3].Info.Kind != "bepinex" {
		t.Errorf("files[3] = %+v, want LogOutput.log/bepinex", files[3].Info)
	}
	if files[3].Path != filepath.Join(paths.BepInExDir(), "LogOutput.log") {
		t.Errorf("files[3].Path = %q, want the BepInEx log path", files[3].Path)
	}
}

func TestListLogFiles_EmptyInstance(t *testing.T) {
	dir := t.TempDir()
	paths := domain.PathsFor(dir, "main")
	if err := os.MkdirAll(paths.Logs, 0o755); err != nil {
		t.Fatal(err)
	}

	files, err := listLogFiles(paths)
	if err != nil {
		t.Fatalf("listLogFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected no log files for a fresh instance, got %+v", files)
	}
}

func TestListLogFiles_IgnoresUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	paths := domain.PathsFor(dir, "main")
	if err := os.MkdirAll(paths.Logs, 0o755); err != nil {
		t.Fatal(err)
	}
	// Not a rotated console log: wrong prefix/suffix.
	writeFile(t, filepath.Join(paths.Logs, "notes.txt"), "irrelevant\n")
	writeFile(t, filepath.Join(paths.Logs, "console-old"), "no .log suffix\n")

	files, err := listLogFiles(paths)
	if err != nil {
		t.Fatalf("listLogFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected unrelated files to be ignored, got %+v", files)
	}
}

func TestSearchLogFiles(t *testing.T) {
	paths := buildLogTree(t)
	files, err := listLogFiles(paths)
	if err != nil {
		t.Fatalf("listLogFiles: %v", err)
	}

	t.Run("plain case-insensitive substring", func(t *testing.T) {
		matches, err := searchLogFiles(files, "LIVE LINE", false, 10)
		if err != nil {
			t.Fatalf("searchLogFiles: %v", err)
		}
		if len(matches) != 2 {
			t.Fatalf("expected 2 matches, got %+v", matches)
		}
		for _, m := range matches {
			if m.File != "console.log" {
				t.Errorf("match %+v not from console.log", m)
			}
		}
	})

	t.Run("regex", func(t *testing.T) {
		matches, err := searchLogFiles(files, `^rotated \w+-name line$`, true, 10)
		if err != nil {
			t.Fatalf("searchLogFiles: %v", err)
		}
		if len(matches) != 2 {
			t.Fatalf("expected 2 regex matches, got %+v", matches)
		}
	})

	t.Run("empty query is a validation error on q", func(t *testing.T) {
		_, err := searchLogFiles(files, "", false, 10)
		assertFieldError(t, err, "q")
	})

	t.Run("bad regex is a validation error on q", func(t *testing.T) {
		_, err := searchLogFiles(files, "(unclosed", true, 10)
		assertFieldError(t, err, "q")
	})

	t.Run("no matches", func(t *testing.T) {
		matches, err := searchLogFiles(files, "nothing-matches-this", false, 10)
		if err != nil {
			t.Fatalf("searchLogFiles: %v", err)
		}
		if len(matches) != 0 {
			t.Errorf("expected no matches, got %+v", matches)
		}
	})
}

func assertFieldError(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	de := domain.AsError(err)
	if de.Code != domain.CodeValidationFailed {
		t.Fatalf("expected CodeValidationFailed, got %v (%v)", de.Code, err)
	}
	for _, f := range de.Fields {
		if f.Field == field {
			return
		}
	}
	t.Fatalf("expected a field error on %q, got %+v", field, de.Fields)
}

// TestSearchLogFiles_NewestFirstAcrossFilesAndStopsAtLimit builds two fake log
// files directly (bypassing listLogFiles) so it can pin exactly which lines
// exist in which scan order, then checks that: matches come out newest-line
// first within a file, the first (newest) file's matches are exhausted before
// the second file contributes anything, and the scan stops once limit is hit
// without ever opening/matching against the remainder.
func TestSearchLogFiles_NewestFirstAcrossFilesAndStopsAtLimit(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.log")
	fileB := filepath.Join(dir, "b.log")
	writeFile(t, fileA, "keep1 apple\nskip\nkeep2 apple\nkeep3 apple\n")
	writeFile(t, fileB, "keepB1 apple\nkeepB2 apple\n")
	files := []logFile{
		{Info: domain.LogFileInfo{Name: "a.log", Kind: "console"}, Path: fileA},
		{Info: domain.LogFileInfo{Name: "b.log", Kind: "rotated"}, Path: fileB},
	}

	t.Run("limit spans both files", func(t *testing.T) {
		matches, err := searchLogFiles(files, "apple", false, 5)
		if err != nil {
			t.Fatalf("searchLogFiles: %v", err)
		}
		wantLines := []string{"keep3 apple", "keep2 apple", "keep1 apple", "keepB2 apple", "keepB1 apple"}
		if len(matches) != len(wantLines) {
			t.Fatalf("got %d matches, want %d: %+v", len(matches), len(wantLines), matches)
		}
		for i, want := range wantLines {
			if matches[i].Line != want {
				t.Errorf("match[%d].Line = %q, want %q", i, matches[i].Line, want)
			}
		}
		if matches[0].File != "a.log" || matches[4].File != "b.log" {
			t.Errorf("expected file A's matches before file B's, got %+v", matches)
		}
	})

	t.Run("limit stops within the first file", func(t *testing.T) {
		matches, err := searchLogFiles(files, "apple", false, 2)
		if err != nil {
			t.Fatalf("searchLogFiles: %v", err)
		}
		wantLines := []string{"keep3 apple", "keep2 apple"}
		if len(matches) != len(wantLines) {
			t.Fatalf("got %+v, want %v", matches, wantLines)
		}
		for i, want := range wantLines {
			if matches[i].Line != want {
				t.Errorf("match[%d].Line = %q, want %q", i, matches[i].Line, want)
			}
			if matches[i].File != "a.log" {
				t.Errorf("match[%d].File = %q, want a.log (limit hit before file B)", i, matches[i].File)
			}
		}
	})
}
