package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
)

// sseSafeRecorder is an http.ResponseWriter+http.Flusher whose buffer is
// protected by a mutex, so a test goroutine can safely read the body while
// the handler goroutine is still writing to it (a plain
// httptest.ResponseRecorder is not safe for that under -race).
type sseSafeRecorder struct {
	mu     sync.Mutex
	header http.Header
	status int
	buf    strings.Builder
}

func newSSESafeRecorder() *sseSafeRecorder { return &sseSafeRecorder{header: http.Header{}} }

func (s *sseSafeRecorder) Header() http.Header { return s.header }

func (s *sseSafeRecorder) WriteHeader(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = code
}

func (s *sseSafeRecorder) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *sseSafeRecorder) Flush() {}

func (s *sseSafeRecorder) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func sseWaitForSubscribers(t *testing.T, bus *events.Bus, n int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if bus.SubscriberCount() >= n {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d subscriber(s), have %d", n, bus.SubscriberCount())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func sseWaitForBodyContains(t *testing.T, rec *sseSafeRecorder, want string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(rec.String(), want) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for body to contain %q; got:\n%s", want, rec.String())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestEventsSSE_StreamsAndFramesEvents(t *testing.T) {
	d := &Deps{
		Cfg: config.Config{},
		Log: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 4})),
		Bus: events.NewBus(),
	}
	router := NewRouter(d, nil) // d.Auth == nil -> devFakeAdmin injects an admin

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
	rec := newSSESafeRecorder()

	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()

	sseWaitForSubscribers(t, d.Bus, 1)

	d.Bus.Publish(domain.Event{Name: domain.EventJobUpdated, Data: map[string]any{"id": "job-1"}})
	d.Bus.Publish(domain.Event{Name: domain.EventInstanceStatus, InstanceID: "inst-1", Data: map[string]any{"state": "running"}})

	sseWaitForBodyContains(t, rec, "event: job.updated")
	sseWaitForBodyContains(t, rec, "event: instance.status")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not exit after client context was cancelled")
	}

	body := rec.String()
	if !strings.Contains(body, "id: 1") {
		t.Fatalf("expected an incrementing id field, got:\n%s", body)
	}
	if !strings.Contains(body, `"id":"job-1"`) {
		t.Fatalf("expected job-1 payload in the stream, got:\n%s", body)
	}
	if !strings.Contains(body, `"state":"running"`) {
		t.Fatalf("expected instance status payload in the stream, got:\n%s", body)
	}
	// Every frame ends with a blank line (event: ...\ndata: ...\n\n).
	if !strings.Contains(body, "\n\n") {
		t.Fatalf("expected SSE frames terminated by a blank line, got:\n%s", body)
	}
	if d.Bus.SubscriberCount() != 0 {
		t.Fatal("expected the subscription to be closed once the handler returns (no goroutine/subscriber leak)")
	}
}

func TestEventsSSE_InstanceFilter(t *testing.T) {
	d := &Deps{
		Cfg: config.Config{},
		Log: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 4})),
		Bus: events.NewBus(),
	}
	router := NewRouter(d, nil)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?instance=inst-a", nil).WithContext(ctx)
	rec := newSSESafeRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()
	sseWaitForSubscribers(t, d.Bus, 1)

	d.Bus.Publish(domain.Event{Name: domain.EventInstanceLog, InstanceID: "inst-b", Data: map[string]any{"line": "should be filtered out"}})
	d.Bus.Publish(domain.Event{Name: domain.EventInstanceLog, InstanceID: "inst-a", Data: map[string]any{"line": "should arrive"}})
	sseWaitForBodyContains(t, rec, "should arrive")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not exit")
	}

	if strings.Contains(rec.String(), "should be filtered out") {
		t.Fatalf("expected instance filter to drop inst-b's event, got:\n%s", rec.String())
	}
}
