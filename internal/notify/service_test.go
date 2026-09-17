package notify

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
)

type fakeSend struct {
	ch domain.NotifyChannel
	m  Message
}

type fakeSender struct {
	mu    sync.Mutex
	calls []fakeSend
}

func (f *fakeSender) Send(_ context.Context, ch domain.NotifyChannel, m Message) error {
	f.mu.Lock()
	f.calls = append(f.calls, fakeSend{ch, m})
	f.mu.Unlock()
	return nil
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type fakeLogRepo struct {
	mu      sync.Mutex
	entries []domain.NotificationLogEntry
}

func (f *fakeLogRepo) Insert(_ context.Context, e domain.NotificationLogEntry) error {
	f.mu.Lock()
	f.entries = append(f.entries, e)
	f.mu.Unlock()
	return nil
}

func (f *fakeLogRepo) List(_ context.Context, limit int, before *time.Time) ([]domain.NotificationLogEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.NotificationLogEntry, len(f.entries))
	copy(out, f.entries)
	return out, nil
}

func (f *fakeLogRepo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.entries)
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func waitFor(t *testing.T, fn func() int, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for count >= %d, got %d", want, fn())
}

func waitForSubscriber(t *testing.T, bus *events.Bus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bus.SubscriberCount() > 0 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the service to subscribe")
}

func crashEvent(instanceID, detail string) domain.Event {
	return domain.Event{
		Name: domain.EventInstanceCrashed, InstanceID: instanceID,
		Data: domain.InstanceEvent{InstanceID: instanceID, Kind: "crash", Detail: detail},
	}
}

func TestService_CrashCooldown_SendsOnce(t *testing.T) {
	bus := events.NewBus()
	sender := &fakeSender{}
	ch := domain.NotifyChannel{ID: "c1", Type: domain.NotifyChannelWebhook, Enabled: true, URL: "http://x", Events: []string{domain.AlertCrashed}}
	settings := func() domain.NotifySettings { return domain.NotifySettings{Channels: []domain.NotifyChannel{ch}} }
	svc := New(bus, settings, t.TempDir(), &fakeLogRepo{}, testLogger(), WithSender(sender))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)
	waitForSubscriber(t, bus)

	bus.Publish(crashEvent("main", "boom"))
	waitFor(t, sender.count, 1)

	bus.Publish(crashEvent("main", "boom again"))
	time.Sleep(50 * time.Millisecond)
	if n := sender.count(); n != 1 {
		t.Fatalf("expected exactly 1 send within the 10 minute cooldown, got %d", n)
	}
}

func TestService_EventNotInChannelEvents_SendsNothing(t *testing.T) {
	bus := events.NewBus()
	sender := &fakeSender{}
	// Subscribed to game_update only, not crashed.
	ch := domain.NotifyChannel{ID: "c1", Type: domain.NotifyChannelWebhook, Enabled: true, URL: "http://x", Events: []string{domain.AlertGameUpdate}}
	settings := func() domain.NotifySettings { return domain.NotifySettings{Channels: []domain.NotifyChannel{ch}} }
	svc := New(bus, settings, t.TempDir(), &fakeLogRepo{}, testLogger(), WithSender(sender))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)
	waitForSubscriber(t, bus)

	bus.Publish(crashEvent("main", "boom"))
	time.Sleep(50 * time.Millisecond)
	if n := sender.count(); n != 0 {
		t.Fatalf("expected no sends for an event kind the channel is not subscribed to, got %d", n)
	}
}

func TestService_DisabledChannel_SendsNothing(t *testing.T) {
	bus := events.NewBus()
	sender := &fakeSender{}
	ch := domain.NotifyChannel{ID: "c1", Type: domain.NotifyChannelWebhook, Enabled: false, URL: "http://x", Events: []string{domain.AlertCrashed}}
	settings := func() domain.NotifySettings { return domain.NotifySettings{Channels: []domain.NotifyChannel{ch}} }
	svc := New(bus, settings, t.TempDir(), &fakeLogRepo{}, testLogger(), WithSender(sender))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)
	waitForSubscriber(t, bus)

	bus.Publish(crashEvent("main", "boom"))
	time.Sleep(50 * time.Millisecond)
	if n := sender.count(); n != 0 {
		t.Fatalf("expected no sends for a disabled channel, got %d", n)
	}
}

func TestService_Test_SendsRegardlessOfEvents(t *testing.T) {
	bus := events.NewBus()
	sender := &fakeSender{}
	logRepo := &fakeLogRepo{}
	// No events configured at all.
	ch := domain.NotifyChannel{ID: "c1", Type: domain.NotifyChannelWebhook, Enabled: false, URL: "http://x", Events: nil}
	settings := func() domain.NotifySettings { return domain.NotifySettings{Channels: []domain.NotifyChannel{ch}} }
	svc := New(bus, settings, t.TempDir(), logRepo, testLogger(), WithSender(sender))

	if err := svc.Test(context.Background(), "c1"); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if n := sender.count(); n != 1 {
		t.Fatalf("expected 1 send, got %d", n)
	}
	if n := logRepo.count(); n != 1 {
		t.Fatalf("expected 1 log entry, got %d", n)
	}
}

func TestService_Test_UnknownChannelIsNotFound(t *testing.T) {
	bus := events.NewBus()
	svc := New(bus, func() domain.NotifySettings { return domain.NotifySettings{} }, t.TempDir(), &fakeLogRepo{}, testLogger(), WithSender(&fakeSender{}))
	err := svc.Test(context.Background(), "does-not-exist")
	if domain.AsError(err).Code != domain.CodeNotFound {
		t.Fatalf("expected not_found, got %v", err)
	}
}

func TestService_InstanceFilter(t *testing.T) {
	bus := events.NewBus()
	sender := &fakeSender{}
	ch := domain.NotifyChannel{
		ID: "c1", Type: domain.NotifyChannelWebhook, Enabled: true, URL: "http://x",
		Events: []string{domain.AlertCrashed}, Instances: []string{"other"},
	}
	settings := func() domain.NotifySettings { return domain.NotifySettings{Channels: []domain.NotifyChannel{ch}} }
	svc := New(bus, settings, t.TempDir(), &fakeLogRepo{}, testLogger(), WithSender(sender))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)
	waitForSubscriber(t, bus)

	bus.Publish(crashEvent("main", "boom")) // channel only watches "other"
	time.Sleep(50 * time.Millisecond)
	if n := sender.count(); n != 0 {
		t.Fatalf("expected no sends for an instance outside the channel's Instances filter, got %d", n)
	}

	bus.Publish(crashEvent("other", "boom"))
	waitFor(t, sender.count, 1)
}

func TestService_DownAlert_OnlyOnStateChange(t *testing.T) {
	bus := events.NewBus()
	sender := &fakeSender{}
	ch := domain.NotifyChannel{ID: "c1", Type: domain.NotifyChannelWebhook, Enabled: true, URL: "http://x", Events: []string{domain.AlertDown}}
	settings := func() domain.NotifySettings { return domain.NotifySettings{Channels: []domain.NotifyChannel{ch}} }
	svc := New(bus, settings, t.TempDir(), &fakeLogRepo{}, testLogger(), WithSender(sender))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)
	waitForSubscriber(t, bus)

	failedEvent := func() domain.Event {
		return domain.Event{Name: domain.EventInstanceStatus, InstanceID: "main", Data: domain.InstanceStatus{InstanceID: "main", State: domain.StateFailed}}
	}
	bus.Publish(failedEvent())
	waitFor(t, sender.count, 1)

	// A second "failed" publish without an intervening state change must not
	// re-alert (this is not the cooldown case: it fires immediately after).
	bus.Publish(failedEvent())
	time.Sleep(50 * time.Millisecond)
	if n := sender.count(); n != 1 {
		t.Fatalf("expected no repeat alert while the state has not changed, got %d", n)
	}
}

func TestService_DiskLow(t *testing.T) {
	bus := events.NewBus()
	sender := &fakeSender{}
	ch := domain.NotifyChannel{ID: "c1", Type: domain.NotifyChannelWebhook, Enabled: true, URL: "http://x", Events: []string{domain.AlertDiskLow}}
	settings := func() domain.NotifySettings {
		return domain.NotifySettings{Channels: []domain.NotifyChannel{ch}, DiskLowPercent: 100} // always "low" for the test
	}
	var now time.Time
	svc := New(bus, settings, t.TempDir(), &fakeLogRepo{}, testLogger(), WithSender(sender), WithClock(func() time.Time { return now }))
	svc.diskCheckInterval = time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)

	waitFor(t, sender.count, 1)

	// The ticker keeps firing every millisecond, but the cooldown must still
	// hold: many more ticks over 100ms must not produce more than one send.
	time.Sleep(100 * time.Millisecond)
	if n := sender.count(); n != 1 {
		t.Fatalf("expected exactly 1 disk_low send within the cooldown despite repeated ticks, got %d", n)
	}
}
