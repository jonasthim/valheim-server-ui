// Package players tracks who is online per Valheim instance (from the console
// log heuristic and A2S_INFO polling), persists the known-players registry,
// and reads/writes the admin/banned/permitted list files
// (ARCHITECTURE.md §8, §14).
package players

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
	"github.com/jonasthim/valheim-server-ui/internal/logs"
	"github.com/jonasthim/valheim-server-ui/internal/query"
)

// a2sFreshWindow is how long a polled A2S result is trusted over the
// log-derived count (ARCHITECTURE.md §8: "authoritative player count comes
// from A2S_INFO ... polled every 15 s").
const a2sFreshWindow = 60 * time.Second

// a2sPollInterval matches ARCHITECTURE.md §8.
const a2sPollInterval = 15 * time.Second

// storeTimeout bounds each database write triggered by a log event so a slow
// or stuck database never blocks log processing indefinitely.
const storeTimeout = 3 * time.Second

// tailerLogBacklog is how many trailing lines of an already-running
// instance's console.log the tracker replays on attach, so a manager restart
// quickly recovers ready/join-code/online state instead of waiting for new
// output.
const tailerLogBacklog = 200

// InstanceRef is what the players package needs to know about one instance
// to attach a tracker to it. Supplied by the instance service (WP-02) through
// the ListInstances callback injected at construction.
type InstanceRef struct {
	ID        string
	Paths     domain.InstancePaths
	QueryPort int
	Running   bool // true when state is running or starting
}

// entry is one currently-online player.
type entry struct {
	id          string // platform id; "" when only known by name
	name        string
	connectedAt time.Time
}

type pendingConn struct {
	id string
	at time.Time
}

// tracker holds the live view of one instance: online players (via the log
// heuristic), the last join code seen, readiness, and the last A2S poll.
type tracker struct {
	instanceID string
	pub        domain.Publisher
	store      PlayerStore
	log        *slog.Logger

	mu       sync.Mutex
	ready    bool
	joinCode string
	online   map[string]*entry
	pending  []pendingConn
	a2s      *domain.A2SInfo
	a2sAt    time.Time
}

func newTracker(instanceID string, pub domain.Publisher, store PlayerStore, log *slog.Logger) *tracker {
	return &tracker{
		instanceID: instanceID,
		pub:        pub,
		store:      store,
		log:        log,
		online:     map[string]*entry{},
	}
}

// HandleLogEvent applies one parsed console.log event to the tracker's state.
func (t *tracker) HandleLogEvent(ctx context.Context, ev logs.Event) {
	switch e := ev.(type) {
	case logs.Ready:
		t.mu.Lock()
		t.ready = true
		t.mu.Unlock()
	case logs.JoinCode:
		t.mu.Lock()
		t.joinCode = e.Code
		t.mu.Unlock()
	case logs.Connected:
		t.onConnected(ctx, e.ID)
	case logs.Spawned:
		t.onSpawned(ctx, e.Name)
	case logs.Disconnected:
		t.onDisconnected(e.ID)
	case logs.Despawned, logs.Saved, logs.Shutdown:
		// No effect on the online set: a despawn is a death/logout
		// transition (not necessarily a disconnect), and Saved/Shutdown
		// carry no player information.
	}
}

// HandleA2S records the latest A2S_INFO poll result.
func (t *tracker) HandleA2S(info domain.A2SInfo) {
	t.mu.Lock()
	info.QueriedAt = time.Now()
	t.a2s = &info
	t.a2sAt = info.QueriedAt
	// The query port is authoritative for the count (ARCHITECTURE.md §8); use
	// it to age out entries the log heuristic could not close. Nobody online
	// clears everything; otherwise only unbound entries are dropped, oldest
	// first, since bound ones will be closed by their own disconnect line.
	changed := false
	switch {
	case info.Players == 0 && len(t.online) > 0:
		t.online = map[string]*entry{}
		changed = true
	case info.Players > 0 && len(t.online) > info.Players:
		changed = t.dropUnboundLocked(len(t.online)-info.Players) > 0
	}
	t.mu.Unlock()
	if changed {
		t.publish()
	}
}

func (t *tracker) onConnected(ctx context.Context, id string) {
	if id == "" {
		return
	}
	t.mu.Lock()
	for _, p := range t.pending {
		if p.id == id {
			t.mu.Unlock()
			return
		}
	}
	// A new connection from an id we still list as online means the previous
	// session ended without a "Closing socket" line we recognised: drop the
	// stale entry and let the coming spawn re-bind it.
	_, wasOnline := t.online["id:"+id]
	delete(t.online, "id:"+id)
	t.pending = append(t.pending, pendingConn{id: id, at: time.Now()})
	t.mu.Unlock()
	if wasOnline {
		t.publish()
	}

	if t.store == nil {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, storeTimeout)
	defer cancel()
	if err := t.store.Upsert(cctx, t.instanceID, id, "", time.Now()); err != nil {
		t.log.Warn("players: record join failed", "instance", t.instanceID, "platform_id", id, "err", err)
	}
}

func (t *tracker) onSpawned(ctx context.Context, name string) {
	t.mu.Lock()
	var boundID string
	connectedAt := time.Now()
	if len(t.pending) > 0 {
		p := t.pending[0]
		t.pending = t.pending[1:]
		boundID = p.id
		connectedAt = p.at
	}
	if boundID == "" {
		// No connection to bind: if this name is already online this is a
		// respawn (death, or a second ZDOID line), not a new player. Creating a
		// second, unbound entry here is what used to leave ghosts behind after
		// the real entry was removed by the disconnect.
		for _, e := range t.online {
			if e.name == name {
				t.mu.Unlock()
				return
			}
		}
	} else {
		// Binding a connection to a name supersedes any unbound entry with
		// that name (e.g. a spawn replayed from the log backlog).
		delete(t.online, "name:"+name)
	}
	key := "name:" + name
	if boundID != "" {
		key = "id:" + boundID
	}
	t.online[key] = &entry{id: boundID, name: name, connectedAt: connectedAt}
	t.mu.Unlock()
	t.publish()

	if boundID == "" || t.store == nil {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, storeTimeout)
	defer cancel()
	if err := t.store.UpdateName(cctx, t.instanceID, boundID, name, time.Now()); err != nil {
		t.log.Warn("players: record name failed", "instance", t.instanceID, "platform_id", boundID, "err", err)
	}
}

func (t *tracker) onDisconnected(id string) {
	if id == "" {
		return
	}
	t.mu.Lock()
	_, matched := t.online["id:"+id]
	delete(t.online, "id:"+id)
	for i, p := range t.pending {
		if p.id == id {
			t.pending = append(t.pending[:i], t.pending[i+1:]...)
			matched = true
			break
		}
	}
	if !matched {
		// The id matched nothing we track, so the player that left is one we
		// only know by name (spawn seen without its connection line). Best
		// effort: drop the longest-connected unbound entry.
		t.dropUnboundLocked(1)
	}
	t.mu.Unlock()
	t.publish()
}

// dropUnboundLocked removes up to n name-only entries, oldest connection
// first. Caller holds t.mu. Returns how many were removed.
func (t *tracker) dropUnboundLocked(n int) int {
	removed := 0
	for removed < n {
		var oldestKey string
		var oldest *entry
		for k, e := range t.online {
			if e.id != "" {
				continue
			}
			if oldest == nil || e.connectedAt.Before(oldest.connectedAt) {
				oldestKey, oldest = k, e
			}
		}
		if oldest == nil {
			break
		}
		delete(t.online, oldestKey)
		removed++
	}
	return removed
}

// reset clears the online set and pending connections (called on Detach: the
// instance stopped, so nobody is connected any more) but keeps the join code
// and last A2S reading for display.
func (t *tracker) reset() {
	t.mu.Lock()
	t.online = map[string]*entry{}
	t.pending = nil
	t.ready = false
	t.mu.Unlock()
	t.publish()
}

func (t *tracker) onlineList() []domain.OnlinePlayer {
	t.mu.Lock()
	out := make([]domain.OnlinePlayer, 0, len(t.online))
	for _, e := range t.online {
		op := domain.OnlinePlayer{PlatformID: e.id, Name: e.name}
		if !e.connectedAt.IsZero() {
			ca := e.connectedAt
			op.ConnectedAt = &ca
		}
		out = append(out, op)
	}
	t.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// snapshot returns everything Enrich/PlayersOnline/the players handler need.
func (t *tracker) snapshot() (online []domain.OnlinePlayer, joinCode string, ready bool, a2s *domain.A2SInfo, a2sAge time.Duration) {
	t.mu.Lock()
	joinCode = t.joinCode
	ready = t.ready
	if t.a2s != nil {
		cp := *t.a2s
		a2s = &cp
		a2sAge = time.Since(t.a2sAt)
	}
	t.mu.Unlock()
	online = t.onlineList()
	return
}

func (t *tracker) publish() {
	if t.pub == nil {
		return
	}
	t.pub.Publish(domain.Event{
		Name:       domain.EventInstancePlayers,
		InstanceID: t.instanceID,
		Data: map[string]any{
			"instance_id": t.instanceID,
			"online":      t.onlineList(),
		},
	})
}

// attachment is what Manager keeps per instance: the tracker (which outlives
// individual attach/detach cycles so join_code survives a restart) and the
// cancel/done pair for the currently-running tail+poll goroutines, if any.
type attachment struct {
	tr     *tracker
	cancel context.CancelFunc
	done   chan struct{}
}

// Manager owns one tracker per instance, tails each running instance's
// console.log, polls its A2S query port, and reacts to instance.status
// events on the bus to attach/detach automatically. It implements
// domain.StatusEnricher and domain.PlayerCounter.
type Manager struct {
	pub     domain.Publisher
	store   PlayerStore
	log     *slog.Logger
	listFn  func(context.Context) ([]InstanceRef, error)
	pathsFn func(string) domain.InstancePaths
	sub     *events.Subscription

	mu          sync.Mutex
	attachments map[string]*attachment
}

// NewManager constructs the manager, bootstraps trackers for every currently
// running/starting instance (via list), subscribes to the bus for
// instance.status transitions, and starts following them — all bound to ctx:
// every goroutine it starts stops when ctx is cancelled. bus may be nil (no
// automatic attach/detach; Attach/Detach must be driven externally).
func NewManager(
	ctx context.Context,
	bus *events.Bus,
	store PlayerStore,
	log *slog.Logger,
	list func(context.Context) ([]InstanceRef, error),
	paths func(string) domain.InstancePaths,
) (*Manager, error) {
	if log == nil {
		log = slog.Default()
	}
	m := &Manager{
		store:       store,
		log:         log,
		listFn:      list,
		pathsFn:     paths,
		attachments: map[string]*attachment{},
	}
	if bus != nil {
		m.pub = bus
	}

	if list != nil {
		refs, err := list(ctx)
		if err != nil {
			return nil, fmt.Errorf("players: bootstrap instance list: %w", err)
		}
		for _, ref := range refs {
			if ref.Running {
				m.Attach(ctx, ref.ID, ref.Paths, ref.QueryPort)
			} else {
				m.getOrCreate(ref.ID) // keep any previously known join code slot
			}
		}
	}

	if bus != nil {
		m.sub = bus.Subscribe("")
		go m.watchStatus(ctx)
	}

	return m, nil
}

func (m *Manager) getOrCreate(id string) *tracker {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.attachments[id]
	if !ok {
		a = &attachment{tr: newTracker(id, m.pub, m.store, m.log)}
		m.attachments[id] = a
	}
	return a.tr
}

func (m *Manager) get(id string) *tracker {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.attachments[id]
	if !ok {
		return nil
	}
	return a.tr
}

// Attach starts tailing paths.ConsoleLog() and polling 127.0.0.1:queryPort
// for instanceID. It is idempotent: calling it again while already attached
// is a no-op.
func (m *Manager) Attach(ctx context.Context, instanceID string, paths domain.InstancePaths, queryPort int) {
	m.mu.Lock()
	a, ok := m.attachments[instanceID]
	if ok && a.cancel != nil {
		m.mu.Unlock()
		return // already running
	}
	if !ok {
		a = &attachment{}
		m.attachments[instanceID] = a
	}
	if a.tr == nil {
		a.tr = newTracker(instanceID, m.pub, m.store, m.log)
	}
	cctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	a.cancel = cancel
	a.done = done
	tr := a.tr
	m.mu.Unlock()

	go m.run(cctx, done, instanceID, tr, paths, queryPort)
}

// Detach stops the tailer and A2S poll loop for instanceID (if attached),
// waits for both goroutines to exit, and clears the online set (keeping the
// join code). Safe to call for an instance that was never attached.
func (m *Manager) Detach(instanceID string) {
	m.mu.Lock()
	a, ok := m.attachments[instanceID]
	if !ok || a.cancel == nil {
		m.mu.Unlock()
		return
	}
	cancel := a.cancel
	done := a.done
	tr := a.tr
	a.cancel = nil
	a.done = nil
	m.mu.Unlock()

	cancel()
	<-done
	tr.reset()
}

func (m *Manager) run(ctx context.Context, done chan struct{}, id string, tr *tracker, paths domain.InstancePaths, queryPort int) {
	defer close(done)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		parse := func(line string) {
			if ev, ok := logs.Parse(line); ok {
				tr.HandleLogEvent(ctx, ev)
			}
		}
		tl := &logs.Tailer{Path: paths.ConsoleLog(), Backlog: tailerLogBacklog, BacklogLine: parse, Logger: m.log}
		if err := tl.Run(ctx, func(line string) {
			parse(line)
			// Live lines feed the console view; backlog lines are not republished
			// because clients already fetched them via GET /logs.
			if m.pub != nil {
				m.pub.Publish(domain.Event{Name: domain.EventInstanceLog, InstanceID: id,
					Data: map[string]string{"instance_id": id, "line": line}})
			}
		}); err != nil {
			m.log.Debug("players: tailer stopped", "instance", id, "err", err)
		}
	}()
	if queryPort > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.pollA2S(ctx, id, tr, queryPort)
		}()
	}
	wg.Wait()
}

func (m *Manager) pollA2S(ctx context.Context, id string, tr *tracker, port int) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	poll := func() {
		info, err := query.Info(ctx, addr)
		if err != nil {
			m.log.Debug("players: a2s query failed", "instance", id, "addr", addr, "err", err)
			return
		}
		tr.HandleA2S(info)
	}
	poll()
	ticker := time.NewTicker(a2sPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}

func (m *Manager) watchStatus(ctx context.Context) {
	defer m.sub.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-m.sub.C:
			if !ok {
				return
			}
			if ev.Name != domain.EventInstanceStatus {
				continue
			}
			st := statusFromEvent(ev.Data)
			if st == nil {
				continue
			}
			switch st.State {
			case domain.StateRunning, domain.StateStarting:
				m.attachFromList(ctx, st.InstanceID)
			case domain.StateStopped, domain.StateFailed:
				m.Detach(st.InstanceID)
			}
		}
	}
}

// attachFromList resolves paths/queryPort for instanceID via listFn (asking
// the instance service, which owns that data) and attaches. If listFn is nil
// or has no entry, it still attaches with paths from pathsFn and A2S polling
// disabled (queryPort 0) rather than dropping the instance's logs entirely.
func (m *Manager) attachFromList(ctx context.Context, instanceID string) {
	var paths domain.InstancePaths
	queryPort := 0
	found := false
	if m.listFn != nil {
		if refs, err := m.listFn(ctx); err == nil {
			for _, ref := range refs {
				if ref.ID == instanceID {
					paths = ref.Paths
					queryPort = ref.QueryPort
					found = true
					break
				}
			}
		} else {
			m.log.Debug("players: list instances failed", "err", err)
		}
	}
	if !found && m.pathsFn != nil {
		paths = m.pathsFn(instanceID)
	}
	m.Attach(ctx, instanceID, paths, queryPort)
}

func statusFromEvent(data any) *domain.InstanceStatus {
	switch v := data.(type) {
	case *domain.InstanceStatus:
		return v
	case domain.InstanceStatus:
		return &v
	default:
		return nil
	}
}

// Snapshot returns the current online list, join code, readiness and last A2S
// reading for instanceID, plus whether it has ever been tracked at all.
func (m *Manager) Snapshot(instanceID string) (online []domain.OnlinePlayer, joinCode string, ready bool, a2s *domain.A2SInfo, a2sAge time.Duration, tracked bool) {
	tr := m.get(instanceID)
	if tr == nil {
		return nil, "", false, nil, 0, false
	}
	online, joinCode, ready, a2s, a2sAge = tr.snapshot()
	return online, joinCode, ready, a2s, a2sAge, true
}

// Enrich implements domain.StatusEnricher.
func (m *Manager) Enrich(_ context.Context, st *domain.InstanceStatus) {
	if st.State != domain.StateRunning && st.State != domain.StateStarting {
		// A stopped/failed process has no live players or session, regardless of
		// what the tracker saw last (detach may still be in flight).
		st.Ready = false
		st.PlayersOnline = 0
		st.JoinCode = ""
		st.A2S = nil
		return
	}
	online, joinCode, ready, a2s, a2sAge, tracked := m.Snapshot(st.InstanceID)
	if !tracked {
		return
	}
	st.Ready = ready
	st.JoinCode = joinCode
	st.A2S = a2s
	if a2s != nil && a2sAge <= a2sFreshWindow {
		st.PlayersOnline = a2s.Players
		st.MaxPlayers = a2s.MaxPlayers
	} else {
		st.PlayersOnline = len(online)
	}
}

// PlayersOnline implements domain.PlayerCounter.
func (m *Manager) PlayersOnline(_ context.Context, instanceID string) (int, bool) {
	online, _, _, a2s, a2sAge, tracked := m.Snapshot(instanceID)
	if !tracked {
		return 0, false
	}
	if a2s != nil && a2sAge <= a2sFreshWindow {
		return a2s.Players, true
	}
	return len(online), true
}
