package jobs

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// logTimeFormat is used for the per-line timestamp prefix written to the job
// log file and included in job.log events. It intentionally differs from the
// job's own RFC3339 timestamps: this is a compact, always-UTC, millisecond
// prefix meant for humans scrolling a console.
const logTimeFormat = "2006-01-02T15:04:05.000Z"

// jobLogPayload is the JSON shape of the `job.log` SSE event
// (docs/openapi.yaml → /events).
type jobLogPayload struct {
	JobID string `json:"job_id"`
	Line  string `json:"line"`
}

// Logger writes one job's output to <jobsDir>/<id>.log and publishes a
// domain.EventJobLog event for every line. It is safe for concurrent use so a
// job's Func can log from multiple goroutines (e.g. exec.Cmd stdout/stderr).
type Logger struct {
	id         string
	instanceID string
	bus        domain.Publisher

	mu      sync.Mutex
	f       *os.File
	lineBuf bytes.Buffer // partial line buffered by the Writer() adapter
	summary map[string]any
}

// newLogger creates (or appends to) <dir>/<id>.log with mode 0640.
func newLogger(dir, id, instanceID string, bus domain.Publisher) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create jobs dir: %w", err)
	}
	path := filepath.Join(dir, id+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640) //nolint:gosec // 0640 is the documented job log permission (ARCHITECTURE.md/WORKPLAN.md WP-04), group-readable for admin tooling
	if err != nil {
		return nil, fmt.Errorf("open job log %s: %w", path, err)
	}
	return &Logger{id: id, instanceID: instanceID, bus: bus, f: f}, nil
}

// Printf formats and appends one line.
func (l *Logger) Printf(format string, args ...any) {
	l.writeLine(fmt.Sprintf(format, args...))
}

// writeLine timestamps, persists and publishes a single line.
func (l *Logger) writeLine(line string) {
	ts := time.Now().UTC().Format(logTimeFormat)
	full := ts + " " + line
	l.mu.Lock()
	f := l.f
	l.mu.Unlock()
	if f != nil {
		_, _ = io.WriteString(f, full+"\n")
	}
	if l.bus != nil {
		l.bus.Publish(domain.Event{
			Name:       domain.EventJobLog,
			InstanceID: l.instanceID,
			Data:       jobLogPayload{JobID: l.id, Line: full},
		})
	}
}

// lineWriter shares Logger's fields so Writer() needs no extra allocation or
// synchronization primitives of its own; it is never used except through the
// (*Logger).Writer() conversion below.
type lineWriter Logger

// Write implements io.Writer, splitting the input into complete lines (each
// becoming one job.log event / log line) and buffering any trailing partial
// line until the next Write or Close.
func (w *lineWriter) Write(p []byte) (int, error) {
	l := (*Logger)(w)
	l.mu.Lock()
	l.lineBuf.Write(p)
	var lines []string
	for {
		b := l.lineBuf.Bytes()
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSuffix(string(b[:i]), "\r")
		lines = append(lines, line)
		l.lineBuf.Next(i + 1)
	}
	l.mu.Unlock()
	for _, line := range lines {
		l.writeLine(line)
	}
	return len(p), nil
}

// Writer returns an io.Writer suitable for exec.Cmd's Stdout/Stderr: it is
// line-buffered and thread-safe (stdout and stderr may both write to it).
func (l *Logger) Writer() io.Writer { return (*lineWriter)(l) }

// SetSummary records one key of the job's final summary (persisted as
// summary_json when the job finishes).
func (l *Logger) SetSummary(key string, v any) {
	l.mu.Lock()
	if l.summary == nil {
		l.summary = map[string]any{}
	}
	l.summary[key] = v
	l.mu.Unlock()
}

// summarySnapshot returns a copy of the accumulated summary, or nil if empty.
func (l *Logger) summarySnapshot() map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.summary) == 0 {
		return nil
	}
	cp := make(map[string]any, len(l.summary))
	for k, v := range l.summary {
		cp[k] = v
	}
	return cp
}

// Close flushes any buffered partial line and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	var leftover string
	if l.lineBuf.Len() > 0 {
		leftover = l.lineBuf.String()
		l.lineBuf.Reset()
	}
	f := l.f
	l.mu.Unlock()
	if leftover != "" {
		l.writeLine(leftover)
	}
	if f == nil {
		return nil
	}
	return f.Close()
}

// readLogFile returns <dir>/<id>.log split into lines. A missing file
// returns an *os.PathError satisfying os.IsNotExist so callers can
// distinguish "never logged anything" from a real read failure.
func readLogFile(dir, id string) ([]string, error) {
	f, err := os.Open(filepath.Join(dir, id+".log"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	lines := []string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan log file: %w", err)
	}
	return lines, nil
}
