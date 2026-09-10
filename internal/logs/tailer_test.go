package logs

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// lineSink is a concurrency-safe collector for Tailer callbacks.
type lineSink struct {
	mu    sync.Mutex
	lines []string
}

func (s *lineSink) add(l string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, l)
}

func (s *lineSink) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.lines...)
}

// tailerWithOpenSignal returns a tailer whose first successful open is
// reported on the channel, so tests write only once writes will be seen.
// A fixed sleep raced the initial open+seek-to-end on slow runners: a late
// open silently skipped whatever the test had already written.
func tailerWithOpenSignal(path string) (*Tailer, <-chan struct{}) {
	opened := make(chan struct{}, 1)
	tl := &Tailer{Path: path}
	tl.onOpen = func() {
		select {
		case opened <- struct{}{}:
		default:
		}
	}
	return tl, opened
}

func awaitOpen(t *testing.T, opened <-chan struct{}) {
	t.Helper()
	select {
	case <-opened:
	case <-time.After(5 * time.Second):
		t.Fatal("tailer did not open the file within 5s")
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func TestTailer_WaitsForFileThenFollows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "console.log")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sink := &lineSink{}
	tl := &Tailer{Path: path}
	done := make(chan error, 1)
	go func() { done <- tl.Run(ctx, sink.add) }()

	// File does not exist yet: nothing should be delivered.
	time.Sleep(150 * time.Millisecond)
	if got := sink.snapshot(); len(got) != 0 {
		t.Fatalf("lines before file exists = %v, want none", got)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.WriteString("line one\nline two\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	waitFor(t, 3*time.Second, func() bool {
		got := sink.snapshot()
		return len(got) == 2 && got[0] == "line one" && got[1] == "line two"
	})

	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("Run returned nil error after cancel, want context.Canceled")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

func TestTailer_AppendConcurrently(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "console.log")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sink := &lineSink{}
	tl, opened := tailerWithOpenSignal(path)
	go func() { _ = tl.Run(ctx, sink.add) }()
	awaitOpen(t, opened)

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	defer f.Close()

	const n = 20
	for i := 0; i < n; i++ {
		if _, err := f.WriteString("appended line\n"); err != nil {
			t.Fatalf("append: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	waitFor(t, 3*time.Second, func() bool { return len(sink.snapshot()) == n })
}

func TestTailer_Backlog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "console.log")
	initial := "l1\nl2\nl3\nl4\nl5\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sink := &lineSink{}
	tl := &Tailer{Path: path, Backlog: 2}
	go func() { _ = tl.Run(ctx, sink.add) }()

	waitFor(t, 2*time.Second, func() bool {
		got := sink.snapshot()
		return len(got) >= 2
	})
	time.Sleep(200 * time.Millisecond) // let it settle without any new writes
	got := sink.snapshot()
	if len(got) != 2 || got[0] != "l4" || got[1] != "l5" {
		t.Fatalf("backlog lines = %v, want [l4 l5]", got)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := f.WriteString("l6\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	waitFor(t, 2*time.Second, func() bool {
		got := sink.snapshot()
		return len(got) == 3 && got[2] == "l6"
	})
}

func TestTailer_Truncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "console.log")
	if err := os.WriteFile(path, []byte("before-1\nbefore-2\n"), 0o644); err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sink := &lineSink{}
	tl := &Tailer{Path: path, Backlog: 10}
	go func() { _ = tl.Run(ctx, sink.add) }()

	waitFor(t, 2*time.Second, func() bool { return len(sink.snapshot()) == 2 })

	if err := os.Truncate(path, 0); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("reopen after truncate: %v", err)
	}
	if _, err := f.WriteString("after-truncate\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	waitFor(t, 2*time.Second, func() bool {
		got := sink.snapshot()
		return len(got) == 3 && got[2] == "after-truncate"
	})
}

func TestTailer_Rotate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "console.log")
	if err := os.WriteFile(path, []byte("old-1\nold-2\n"), 0o644); err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sink := &lineSink{}
	tl := &Tailer{Path: path, Backlog: 10}
	go func() { _ = tl.Run(ctx, sink.add) }()

	waitFor(t, 2*time.Second, func() bool { return len(sink.snapshot()) == 2 })

	// Simulate rotation: rename the old file away, create a fresh one at the
	// original path (as the manager does before starting the instance again).
	rotated := filepath.Join(dir, "console-rotated.log")
	if err := os.Rename(path, rotated); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := os.WriteFile(path, []byte("new-1\n"), 0o644); err != nil {
		t.Fatalf("create new file: %v", err)
	}

	waitFor(t, 3*time.Second, func() bool {
		got := sink.snapshot()
		return len(got) == 3 && got[2] == "new-1"
	})

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("append to new file: %v", err)
	}
	if _, err := f.WriteString("new-2\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	waitFor(t, 2*time.Second, func() bool {
		got := sink.snapshot()
		return len(got) == 4 && got[3] == "new-2"
	})
}

func TestTailer_PartialLineTolerated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "console.log")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sink := &lineSink{}
	tl, opened := tailerWithOpenSignal(path)
	go func() { _ = tl.Run(ctx, sink.add) }()
	awaitOpen(t, opened)

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString("partial-no-newline-yet"); err != nil {
		t.Fatalf("write: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if got := sink.snapshot(); len(got) != 0 {
		t.Fatalf("lines before newline = %v, want none", got)
	}

	if _, err := f.WriteString(" now complete\nsecond\n"); err != nil {
		t.Fatalf("write rest: %v", err)
	}

	// 5 s covers the 1 s poll fallback when fsnotify is late on a loaded runner.
	waitFor(t, 5*time.Second, func() bool {
		got := sink.snapshot()
		return len(got) == 2 && got[0] == "partial-no-newline-yet now complete" && got[1] == "second"
	})
}
