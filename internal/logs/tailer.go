// Package logs follows an instance's console.log and parses it into typed
// events (ARCHITECTURE.md §8). It never fails on unexpected input: unknown
// lines are simply not turned into events.
package logs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// maxPartialLine bounds the in-memory buffer for a line that never terminates
// with \n, so a pathological file cannot grow the tailer's memory forever.
const maxPartialLine = 1 << 20 // 1 MiB

// pollInterval is the fsnotify fallback / dir-existence poll cadence.
const pollInterval = time.Second

// Tailer follows a file from its end (or replays the last Backlog lines
// first), tolerating the file not existing yet, truncation, and rotation
// (rename-away then a fresh file created at the same path). It uses fsnotify
// when available and always falls back to polling every second.
type Tailer struct {
	// Path is the file to follow. It does not need to exist yet.
	Path string
	// Backlog, when > 0, makes Run call onLine with up to this many of the
	// file's existing lines before it starts following new writes.
	Backlog int
	// Logger receives best-effort diagnostics. Defaults to slog.Default().
	Logger *slog.Logger

	f      *os.File
	ino    fileIdentity
	offset int64
	buf    []byte
	// wasMissing tracks whether Path has been observed absent since the last
	// open. When the file had to be waited for, the first open always starts
	// at offset 0 (there is no meaningful "backlog" or "end" to skip to: any
	// bytes written before we got around to opening it are new, not old).
	wasMissing bool
}

// Run follows Path, calling onLine for every complete line (the trailing \n
// stripped, and a trailing \r stripped too), until ctx is done. It returns
// ctx.Err() on cancellation and never panics or calls onLine after returning.
func (t *Tailer) Run(ctx context.Context, onLine func(line string)) error {
	log := t.logger()
	dir := filepath.Dir(t.Path)

	watcher, werr := fsnotify.NewWatcher()
	if werr != nil {
		log.Debug("logs: fsnotify unavailable, polling only", "err", werr)
		watcher = nil
	} else {
		defer func() { _ = watcher.Close() }()
	}
	watchDir := func() {
		if watcher == nil {
			return
		}
		if err := watcher.Add(dir); err != nil {
			log.Debug("logs: watch dir failed", "dir", dir, "err", err)
		}
	}
	watchDir()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	defer func() {
		if t.f != nil {
			_ = t.f.Close()
			t.f = nil
		}
	}()

	relevant := func(name string) bool {
		if name == "" {
			return true // synthetic wake (ticker) — always relevant
		}
		a, err1 := filepath.Abs(name)
		b, err2 := filepath.Abs(t.Path)
		return err1 == nil && err2 == nil && a == b
	}

	for {
		if err := t.check(onLine, log); err != nil {
			log.Debug("logs: tail check failed", "path", t.Path, "err", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			watchDir()
		case ev, ok := <-eventsChan(watcher):
			if !ok {
				continue
			}
			if !relevant(ev.Name) {
				continue
			}
			// Adding the watch again after the directory itself just
			// appeared is a cheap no-op once it already succeeded.
			watchDir()
		case err, ok := <-errorsChan(watcher):
			if ok {
				log.Debug("logs: watcher error", "err", err)
			}
		}
	}
}

// eventsChan/errorsChan tolerate a nil watcher (no fsnotify available).
func eventsChan(w *fsnotify.Watcher) chan fsnotify.Event {
	if w == nil {
		return nil
	}
	return w.Events
}

func errorsChan(w *fsnotify.Watcher) chan error {
	if w == nil {
		return nil
	}
	return w.Errors
}

// check performs one poll iteration: open the file if needed (emitting
// backlog), detect rotation/truncation, and deliver any new complete lines.
func (t *Tailer) check(onLine func(string), log *slog.Logger) error {
	st, err := os.Stat(t.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.wasMissing = true
			if t.f != nil {
				_ = t.f.Close()
				t.f = nil
				t.buf = t.buf[:0]
			}
			return nil
		}
		return fmt.Errorf("stat %s: %w", t.Path, err)
	}

	curIno := identify(st)

	if t.f == nil {
		f, err := os.Open(t.Path)
		if err != nil {
			return fmt.Errorf("open %s: %w", t.Path, err)
		}
		t.f = f
		t.ino = curIno
		t.buf = t.buf[:0]
		if t.wasMissing {
			// The file did not exist a moment ago: whatever is in it now was
			// written no earlier than "now" from our point of view, so start
			// from the beginning instead of skipping to the end or applying
			// Backlog (there is nothing meaningful to treat as history yet).
			t.offset = 0
			t.wasMissing = false
			return nil
		}
		if t.Backlog > 0 {
			lines, size, err := readLastLines(f, t.Backlog)
			if err != nil {
				log.Debug("logs: backlog read failed", "path", t.Path, "err", err)
				size, err = f.Seek(0, io.SeekEnd)
				if err != nil {
					return fmt.Errorf("seek %s: %w", t.Path, err)
				}
			} else {
				for _, l := range lines {
					onLine(l)
				}
				if _, err := f.Seek(size, io.SeekStart); err != nil {
					return fmt.Errorf("seek %s: %w", t.Path, err)
				}
			}
			t.offset = size
		} else {
			off, err := f.Seek(0, io.SeekEnd)
			if err != nil {
				return fmt.Errorf("seek %s: %w", t.Path, err)
			}
			t.offset = off
		}
		return nil
	}

	if curIno != t.ino {
		// Rotated: the path now refers to a different file. Follow the new
		// one from the start.
		_ = t.f.Close()
		f, err := os.Open(t.Path)
		if err != nil {
			t.f = nil
			return fmt.Errorf("reopen %s: %w", t.Path, err)
		}
		t.f = f
		t.ino = curIno
		t.offset = 0
		t.buf = t.buf[:0]
	} else if st.Size() < t.offset {
		// Truncated in place.
		if _, err := t.f.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("seek %s: %w", t.Path, err)
		}
		t.offset = 0
		t.buf = t.buf[:0]
	}

	return t.drain(onLine)
}

// drain reads everything currently available from t.f and emits complete
// lines, buffering any trailing partial line for the next call.
func (t *Tailer) drain(onLine func(string)) error {
	chunk := make([]byte, 32*1024)
	for {
		n, rerr := t.f.Read(chunk)
		if n > 0 {
			t.offset += int64(n)
			t.buf = append(t.buf, chunk[:n]...)
			for {
				i := bytes.IndexByte(t.buf, '\n')
				if i < 0 {
					break
				}
				line := strings.TrimSuffix(string(t.buf[:i]), "\r")
				onLine(line)
				t.buf = t.buf[i+1:]
			}
			if len(t.buf) > maxPartialLine {
				onLine(string(t.buf))
				t.buf = t.buf[:0]
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return nil
			}
			return fmt.Errorf("read %s: %w", t.Path, rerr)
		}
		if n == 0 {
			return nil
		}
	}
}

func (t *Tailer) logger() *slog.Logger {
	if t.Logger != nil {
		return t.Logger
	}
	return slog.Default()
}

// fileIdentity distinguishes the file backing an open path across rotations.
type fileIdentity struct {
	dev, ino uint64
}

func identify(fi os.FileInfo) fileIdentity {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return fileIdentity{dev: st.Dev, ino: st.Ino}
	}
	return fileIdentity{}
}

// readLastLines returns up to n trailing lines of f (which must be
// positioned anywhere; it seeks internally) and the file size at read time
// (the offset callers should continue tailing from).
func readLastLines(f *os.File, n int) ([]string, int64, error) {
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, 0, err
	}
	if n <= 0 || size == 0 {
		return nil, size, nil
	}
	const chunkSize = 64 * 1024
	var data []byte
	pos := size
	for pos > 0 {
		readSize := int64(chunkSize)
		if readSize > pos {
			readSize = pos
		}
		pos -= readSize
		buf := make([]byte, readSize)
		if _, err := f.ReadAt(buf, pos); err != nil && !errors.Is(err, io.EOF) {
			return nil, size, err
		}
		data = append(buf, data...)
		if bytes.Count(data, []byte("\n")) > n {
			break
		}
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil, size, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines, size, nil
}
