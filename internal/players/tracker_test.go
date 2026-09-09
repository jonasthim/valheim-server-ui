package players

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/logs"
)

// fakePublisher records every published event for assertions.
type fakePublisher struct {
	mu     sync.Mutex
	events []domain.Event
}

func (p *fakePublisher) Publish(ev domain.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
}

func (p *fakePublisher) last() (domain.Event, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.events) == 0 {
		return domain.Event{}, false
	}
	return p.events[len(p.events)-1], true
}

func (p *fakePublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

// fakeStore is an in-memory PlayerStore for tests that want to assert on
// persistence without a real database.
type fakeStore struct {
	mu    sync.Mutex
	rows  map[[2]string]domain.KnownPlayer
	order map[string][]string // instanceID -> platform ids in first-seen order
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[[2]string]domain.KnownPlayer{}, order: map[string][]string{}}
}

func (s *fakeStore) Upsert(_ context.Context, instanceID, platformID, name string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := [2]string{instanceID, platformID}
	kp, ok := s.rows[key]
	if !ok {
		kp = domain.KnownPlayer{PlatformID: platformID, FirstSeenAt: now}
		s.order[instanceID] = append(s.order[instanceID], platformID)
	}
	kp.LastSeenAt = now
	kp.SessionCount++
	if name != "" {
		kp.Name = name
	}
	s.rows[key] = kp
	return nil
}

func (s *fakeStore) UpdateName(_ context.Context, instanceID, platformID, name string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := [2]string{instanceID, platformID}
	kp := s.rows[key]
	kp.PlatformID = platformID
	kp.Name = name
	kp.LastSeenAt = now
	if kp.FirstSeenAt.IsZero() {
		kp.FirstSeenAt = now
	}
	s.rows[key] = kp
	return nil
}

func (s *fakeStore) List(_ context.Context, instanceID string, limit int) ([]domain.KnownPlayer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.KnownPlayer
	for _, id := range s.order[instanceID] {
		out = append(out, s.rows[[2]string{instanceID, id}])
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestTracker_ConnectThenSpawnBindsName(t *testing.T) {
	pub := &fakePublisher{}
	store := newFakeStore()
	tr := newTracker("main", pub, store, testLogger())
	ctx := context.Background()

	tr.HandleLogEvent(ctx, logs.Connected{ID: "76561198000000001"})
	online, _, _, _, _ := tr.snapshot()
	if len(online) != 0 {
		t.Fatalf("online after Connected = %v, want empty (still pending)", online)
	}

	tr.HandleLogEvent(ctx, logs.Spawned{Name: "Bjorn"})
	online, _, _, _, _ = tr.snapshot()
	if len(online) != 1 {
		t.Fatalf("online after Spawned = %v, want 1 entry", online)
	}
	if online[0].Name != "Bjorn" || online[0].PlatformID != "76561198000000001" {
		t.Fatalf("online[0] = %#v, want Bjorn/76561198000000001", online[0])
	}

	kp, err := store.List(ctx, "main", 10)
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	if len(kp) != 1 || kp[0].Name != "Bjorn" || kp[0].SessionCount != 1 {
		t.Fatalf("known players = %#v, want one Bjorn with session_count 1", kp)
	}
}

func TestTracker_DuplicateConnectedLinesDedupe(t *testing.T) {
	// Valheim logs both "Got connection SteamID" and "Got handshake from
	// client" for the same id; the tracker must not queue two pending
	// connections for one player (which would misbind the next two spawns).
	pub := &fakePublisher{}
	store := newFakeStore()
	tr := newTracker("main", pub, store, testLogger())
	ctx := context.Background()

	tr.HandleLogEvent(ctx, logs.Connected{ID: "id-1"})
	tr.HandleLogEvent(ctx, logs.Connected{ID: "id-1"}) // duplicate, e.g. handshake line
	tr.HandleLogEvent(ctx, logs.Spawned{Name: "Solo"})
	tr.HandleLogEvent(ctx, logs.Spawned{Name: "Unrelated"}) // no pending connection left

	online, _, _, _, _ := tr.snapshot()
	if len(online) != 2 {
		t.Fatalf("online = %#v, want 2 entries (Solo bound, Unrelated unbound)", online)
	}
	byName := map[string]domain.OnlinePlayer{}
	for _, p := range online {
		byName[p.Name] = p
	}
	if byName["Solo"].PlatformID != "id-1" {
		t.Errorf("Solo platform id = %q, want id-1", byName["Solo"].PlatformID)
	}
	if byName["Unrelated"].PlatformID != "" {
		t.Errorf("Unrelated platform id = %q, want empty (no pending connection)", byName["Unrelated"].PlatformID)
	}

	kp, _ := store.List(ctx, "main", 10)
	if len(kp) != 1 {
		t.Fatalf("known players = %#v, want exactly one row (Upsert de-duplicated)", kp)
	}
}

func TestTracker_DisconnectRemovesByID(t *testing.T) {
	pub := &fakePublisher{}
	tr := newTracker("main", pub, newFakeStore(), testLogger())
	ctx := context.Background()

	tr.HandleLogEvent(ctx, logs.Connected{ID: "id-1"})
	tr.HandleLogEvent(ctx, logs.Spawned{Name: "Bjorn"})
	online, _, _, _, _ := tr.snapshot()
	if len(online) != 1 {
		t.Fatalf("online before disconnect = %v, want 1", online)
	}

	tr.HandleLogEvent(ctx, logs.Disconnected{ID: "id-1"})
	online, _, _, _, _ = tr.snapshot()
	if len(online) != 0 {
		t.Fatalf("online after disconnect = %v, want empty", online)
	}
}

func TestTracker_SpawnWithoutConnectAddsUnboundEntry(t *testing.T) {
	pub := &fakePublisher{}
	tr := newTracker("main", pub, newFakeStore(), testLogger())
	ctx := context.Background()

	tr.HandleLogEvent(ctx, logs.Spawned{Name: "Ghost"})
	online, _, _, _, _ := tr.snapshot()
	if len(online) != 1 || online[0].Name != "Ghost" || online[0].PlatformID != "" {
		t.Fatalf("online = %#v, want one unbound Ghost", online)
	}
}

func TestTracker_ReadyAndJoinCode(t *testing.T) {
	tr := newTracker("main", &fakePublisher{}, newFakeStore(), testLogger())
	ctx := context.Background()

	_, joinCode, ready, _, _ := tr.snapshot()
	if ready || joinCode != "" {
		t.Fatalf("initial state = ready=%v joinCode=%q, want false/empty", ready, joinCode)
	}

	tr.HandleLogEvent(ctx, logs.Ready{})
	tr.HandleLogEvent(ctx, logs.JoinCode{Code: "123456", Players: 0})

	_, joinCode, ready, _, _ = tr.snapshot()
	if !ready || joinCode != "123456" {
		t.Fatalf("state = ready=%v joinCode=%q, want true/123456", ready, joinCode)
	}
}

func TestTracker_A2SPrecedenceWhenFresh(t *testing.T) {
	tr := newTracker("main", &fakePublisher{}, newFakeStore(), testLogger())
	ctx := context.Background()

	// Two players known from the log heuristic...
	tr.HandleLogEvent(ctx, logs.Connected{ID: "id-1"})
	tr.HandleLogEvent(ctx, logs.Spawned{Name: "A"})
	tr.HandleLogEvent(ctx, logs.Connected{ID: "id-2"})
	tr.HandleLogEvent(ctx, logs.Spawned{Name: "B"})

	// ...but A2S (crossplay players included) reports 5.
	tr.HandleA2S(domain.A2SInfo{Players: 5, MaxPlayers: 10})

	online, _, _, a2s, a2sAge := tr.snapshot()
	if len(online) != 2 {
		t.Fatalf("online = %v, want 2 log-tracked entries", online)
	}
	if a2s == nil || a2s.Players != 5 {
		t.Fatalf("a2s = %#v, want Players=5", a2s)
	}
	if a2sAge > a2sFreshWindow {
		t.Fatalf("a2sAge = %s, want within freshness window", a2sAge)
	}
}

func TestTracker_ResetClearsOnlineKeepsJoinCode(t *testing.T) {
	tr := newTracker("main", &fakePublisher{}, newFakeStore(), testLogger())
	ctx := context.Background()
	tr.HandleLogEvent(ctx, logs.JoinCode{Code: "999999", Players: 0})
	tr.HandleLogEvent(ctx, logs.Connected{ID: "id-1"})
	tr.HandleLogEvent(ctx, logs.Spawned{Name: "Bjorn"})

	tr.reset()

	online, joinCode, ready, _, _ := tr.snapshot()
	if len(online) != 0 {
		t.Fatalf("online after reset = %v, want empty", online)
	}
	if joinCode != "999999" {
		t.Fatalf("joinCode after reset = %q, want kept as 999999", joinCode)
	}
	if ready {
		t.Fatalf("ready after reset = true, want false")
	}
}

func TestTracker_PublishesOnOnlineSetChange(t *testing.T) {
	pub := &fakePublisher{}
	tr := newTracker("main", pub, newFakeStore(), testLogger())
	ctx := context.Background()

	tr.HandleLogEvent(ctx, logs.Connected{ID: "id-1"}) // no online-set change yet
	if pub.count() != 0 {
		t.Fatalf("publish count after Connected = %d, want 0", pub.count())
	}

	tr.HandleLogEvent(ctx, logs.Spawned{Name: "Bjorn"})
	ev, ok := pub.last()
	if !ok {
		t.Fatalf("no event published after Spawned")
	}
	if ev.Name != domain.EventInstancePlayers || ev.InstanceID != "main" {
		t.Fatalf("event = %#v, want instance.players for main", ev)
	}
	payload, ok := ev.Data.(map[string]any)
	if !ok {
		t.Fatalf("event.Data = %#v, want map[string]any", ev.Data)
	}
	if payload["instance_id"] != "main" {
		t.Fatalf("payload instance_id = %v, want main", payload["instance_id"])
	}
}

func TestManager_AttachDetachStopsGoroutines(t *testing.T) {
	dir := t.TempDir()
	paths := domain.InstancePaths{Root: dir, Logs: dir}
	if err := os.WriteFile(filepath.Join(dir, "console.log"), nil, 0o644); err != nil {
		t.Fatalf("seed console.log: %v", err)
	}

	mgr, err := NewManager(context.Background(), nil, newFakeStore(), testLogger(), nil, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Attach(ctx, "main", paths, 0) // queryPort 0: no A2S polling, just the tailer

	f, err := os.OpenFile(filepath.Join(dir, "console.log"), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	if _, err := f.WriteString("09/09/2026 10:00:00: Game server connected\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, _, ready, _, _, _ := mgr.Snapshot("main")
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tracker never observed Ready")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Detach must block until the tailer goroutine has actually exited: after
	// it returns, further writes to the file must never reach the tracker.
	mgr.Detach("main")

	f2, err := os.OpenFile(filepath.Join(dir, "console.log"), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	if _, err := f2.WriteString("09/09/2026 10:00:01: Session \"x\" with join code 555555 and IP 1.2.3.4:2456 is active with 0 player(s)\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f2.Close()

	time.Sleep(200 * time.Millisecond)
	_, joinCode, ready, _, _, _ := mgr.Snapshot("main")
	if ready {
		t.Errorf("ready after Detach = true, want false (reset)")
	}
	if joinCode != "" {
		t.Errorf("joinCode after Detach+write = %q, want empty: the detached tailer must not still be running", joinCode)
	}

	// Detaching an instance that was never attached must be a safe no-op.
	mgr.Detach("never-attached")
}

func TestManager_BootstrapsFromListAndEnriches(t *testing.T) {
	dir := t.TempDir()
	paths := domain.InstancePaths{Root: dir, Logs: dir}
	if err := os.WriteFile(filepath.Join(dir, "console.log"),
		[]byte("09/09/2026 10:00:00: Game server connected\n"+
			"09/09/2026 10:00:01: Session \"x\" with join code 123456 and IP 1.2.3.4:2456 is active with 0 player(s)\n"),
		0o644); err != nil {
		t.Fatalf("seed console.log: %v", err)
	}

	list := func(context.Context) ([]InstanceRef, error) {
		return []InstanceRef{{ID: "main", Paths: paths, QueryPort: 0, Running: true}}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr, err := NewManager(ctx, nil, newFakeStore(), testLogger(), list, func(string) domain.InstancePaths { return paths })
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		st := &domain.InstanceStatus{InstanceID: "main"}
		mgr.Enrich(ctx, st)
		if st.Ready && st.JoinCode == "123456" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Enrich never reflected bootstrap state: ready=%v joinCode=%q", st.Ready, st.JoinCode)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, known := mgr.PlayersOnline(ctx, "unknown-instance"); known {
		t.Errorf("PlayersOnline for untracked instance reported known=true")
	}
	if n, known := mgr.PlayersOnline(ctx, "main"); !known || n != 0 {
		t.Errorf("PlayersOnline(main) = %d,%v want 0,true", n, known)
	}
}
