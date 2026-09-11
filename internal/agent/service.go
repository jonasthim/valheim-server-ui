package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// pollInterval is how often running instances are asked for their status.
const pollInterval = 2 * time.Second

// publishEvery bounds how often an unchanged-but-connected status is
// re-published (players moving still count as a change).
const publishEvery = 10 * time.Second

// InstanceSource is the slice of the instance service the agent needs.
type InstanceSource interface {
	List(ctx context.Context) ([]domain.Instance, error)
	Get(ctx context.Context, id string) (*domain.Instance, error)
	Paths(id string) domain.InstancePaths
}

// Versioner reports the plugin version this manager ships (Bundle).
type Versioner interface {
	Version() string
}

type state struct {
	explored        *domain.ExploredInfo
	connected       bool
	lastSeen        time.Time
	lastError       string
	status          *domain.AgentStatus
	lastPublished   time.Time
	lastFingerprint string
}

// Service polls agents, enriches instance status and executes commands.
type Service struct {
	inst    InstanceSource
	bus     domain.Publisher
	log     *slog.Logger
	bundle  Versioner
	http    *http.Client
	baseURL func(port int) string // tests point this at a fake agent
	mu      sync.Mutex
	states  map[string]*state
	maps    map[string]*mapState
	now     func() time.Time
}

// NewService wires the agent integration. bundle may be nil (no bundled
// version known).
func NewService(inst InstanceSource, bus domain.Publisher, log *slog.Logger, bundle Versioner) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		inst: inst, bus: bus, log: log, bundle: bundle,
		http:   &http.Client{Timeout: 3 * time.Second},
		states: map[string]*state{},
		maps:   map[string]*mapState{},
		now:    time.Now,
	}
}

// PreStart is registered with the instance service: right before an instance
// starts it writes the plugin config (port and token) when the agent is
// installed and BepInEx is enabled, so the plugin comes up reachable.
func (s *Service) PreStart(_ context.Context, id string, paths domain.InstancePaths, cfg domain.InstanceConfig) error {
	if !cfg.BepInExEnabled || !Installed(paths) {
		return nil
	}
	if _, err := EnsureConfig(paths, cfg.Port); err != nil {
		return err
	}
	s.log.Debug("agent: config written", "instance", id, "port", cfg.Port)
	return nil
}

// Enrich implements domain.StatusEnricher.
func (s *Service) Enrich(_ context.Context, st *domain.InstanceStatus) {
	if st == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.states[st.InstanceID]; ok {
		st.AgentConnected = x.connected
	}
}

// Run polls until ctx ends.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

// tick polls every running instance that has the agent installed.
func (s *Service) tick(ctx context.Context) {
	list, err := s.inst.List(ctx)
	if err != nil {
		s.log.Debug("agent: list instances", "err", err)
		return
	}
	seen := map[string]bool{}
	for i := range list {
		in := &list[i]
		seen[in.ID] = true
		running := in.Status.State == domain.StateRunning || in.Status.State == domain.StateStarting
		paths := s.inst.Paths(in.ID)
		if !running || !in.Config.BepInExEnabled || !Installed(paths) {
			s.setDisconnected(in.ID, "")
			continue
		}
		s.poll(ctx, in.ID, in.Config.Port, paths)
	}
	s.mu.Lock()
	for id := range s.states {
		if !seen[id] {
			delete(s.states, id)
			delete(s.maps, id)
		}
	}
	s.mu.Unlock()
}

func (s *Service) client(paths domain.InstancePaths, port int) (*Client, error) {
	cfg, err := ReadConfig(paths)
	if err != nil {
		return nil, err
	}
	if cfg.Token == "" {
		return nil, domain.E(domain.CodeConflict, "agent config has no token yet; restart the instance")
	}
	if cfg.Port > 0 {
		port = cfg.Port
	}
	if s.baseURL != nil {
		return NewClientForURL(s.baseURL(port), cfg.Token, s.http), nil
	}
	return NewClient(port, cfg.Token, s.http), nil
}

func (s *Service) poll(ctx context.Context, id string, port int, paths domain.InstancePaths) {
	c, err := s.client(paths, port)
	if err != nil {
		s.setDisconnected(id, err.Error())
		return
	}
	pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	st, err := c.Status(pctx)
	cancel()
	if err != nil {
		s.setDisconnected(id, err.Error())
		return
	}
	// Fog state rides along so the map hears about new masks within a
	// poll instead of its own slower refresh.
	s.mu.Lock()
	ms := s.maps[id]
	if ms == nil {
		ms = &mapState{}
		s.maps[id] = ms
	}
	fogKnownUnsupported := ms.fogUnsupported || ms.unsupported
	s.mu.Unlock()
	var explored *domain.ExploredInfo
	if !fogKnownUnsupported {
		ectx, ecancel := context.WithTimeout(ctx, 2*time.Second)
		ei, eerr := c.ExploredInfo(ectx)
		ecancel()
		s.mu.Lock()
		switch {
		case eerr == nil:
			ms.explored, ms.exploredAt = ei, s.now()
			explored = ei
		case isNotFound(eerr):
			ms.fogUnsupported = true
		default:
			explored = ms.explored
		}
		s.mu.Unlock()
	}
	s.mu.Lock()
	x := s.states[id]
	if x == nil {
		x = &state{}
		s.states[id] = x
	}
	x.explored = explored
	changed := !x.connected
	x.connected = true
	x.lastError = ""
	x.lastSeen = s.now()
	x.status = st
	fp := fingerprint(st)
	if explored != nil {
		// Exploration progress (the percent in the UI) and the mask the
		// fogged image is built from each count as a change worth an event.
		fp += fmt.Sprintf("|expl:%d/%d|mask:%d", explored.Version, explored.ExploredCells, explored.MaskVersion)
	}
	if fp != x.lastFingerprint {
		changed = true
		x.lastFingerprint = fp
	}
	due := s.now().Sub(x.lastPublished) >= publishEvery
	var info *domain.AgentInfo
	if changed || due {
		x.lastPublished = s.now()
		info = s.infoLocked(id, paths, x, false)
	}
	s.mu.Unlock()
	if info != nil {
		s.publish(id, info)
	}
}

func (s *Service) setDisconnected(id, reason string) {
	s.mu.Lock()
	x := s.states[id]
	if x == nil {
		if reason == "" {
			s.mu.Unlock()
			return
		}
		x = &state{}
		s.states[id] = x
	}
	was := x.connected
	x.connected = false
	x.lastError = reason
	x.status = nil
	x.lastFingerprint = ""
	var info *domain.AgentInfo
	if was {
		info = s.infoLocked(id, s.inst.Paths(id), x, false)
	}
	s.mu.Unlock()
	if info != nil {
		s.publish(id, info)
	}
}

func (s *Service) publish(id string, info *domain.AgentInfo) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(domain.Event{Name: domain.EventAgentStatus, InstanceID: id, Data: map[string]any{
		"instance_id": id,
		"agent":       info,
	}})
}

// fingerprint summarises what the UI cares about so unchanged polls are not
// re-broadcast: player set and rounded positions, day, weather.
func fingerprint(st *domain.AgentStatus) string {
	type p struct {
		U    int64
		N    string
		X, Z int
	}
	ps := make([]p, 0, len(st.Players))
	for _, pl := range st.Players {
		e := p{U: pl.UID, N: pl.Name}
		if pl.Position != nil {
			e.X, e.Z = int(pl.Position.X), int(pl.Position.Z)
		}
		ps = append(ps, e)
	}
	var lastPing int64
	if n := len(st.Pings); n > 0 {
		lastPing = st.Pings[n-1].At.UnixMilli()
	}
	b, _ := json.Marshal(struct {
		Ready bool
		Day   int
		W     string
		Keys  int
		P     []p
		Pings int
		Ping  int64
	}{st.Ready, st.World.Day, st.World.Weather, len(st.GlobalKeys), ps, len(st.Pings), lastPing})
	return string(b)
}

// Info reports the agent's state for id. includeHidden keeps positions of
// players who hide their map position (operator and above).
func (s *Service) Info(ctx context.Context, id string, includeHidden bool) (*domain.AgentInfo, error) {
	in, err := s.inst.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	paths := s.inst.Paths(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	x := s.states[id]
	if x == nil {
		x = &state{}
	}
	info := s.infoLocked(id, paths, x, includeHidden)
	info.Enabled = in.Config.BepInExEnabled
	return info, nil
}

func (s *Service) infoLocked(_ string, paths domain.InstancePaths, x *state, includeHidden bool) *domain.AgentInfo {
	info := &domain.AgentInfo{
		Installed: Installed(paths),
		Connected: x.connected,
		LastError: x.lastError,
	}
	if !x.lastSeen.IsZero() {
		t := x.lastSeen
		info.LastSeen = &t
	}
	if info.Installed {
		info.InstalledVersion = installedVersion(paths)
	}
	if s.bundle != nil {
		info.BundledVersion = s.bundle.Version()
		info.UpdateAvailable = info.Installed && info.BundledVersion != "0.0.0" && info.InstalledVersion != "" && info.InstalledVersion != info.BundledVersion
	}
	if x.explored != nil {
		ei := *x.explored
		info.Explored = &ei
	}
	if x.status != nil {
		st := *x.status
		normalizeStatus(&st)
		st.Players = make([]domain.AgentPlayer, 0, len(x.status.Players))
		for _, p := range x.status.Players {
			if !p.Visible && !includeHidden {
				p.Position = nil
			}
			st.Players = append(st.Players, p)
		}
		info.Status = &st
	}
	return info
}

// installedVersion reads version_number from the plugin's manifest.json.
func installedVersion(paths domain.InstancePaths) string {
	b, err := os.ReadFile(filepath.Join(PluginDir(paths), "manifest.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Version string `json:"version_number"`
	}
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	return m.Version
}

// Command forwards an admin command to id's agent.
func (s *Service) Command(ctx context.Context, id string, req domain.AgentCommandRequest) (*domain.AgentCommandResult, error) {
	known := false
	for _, c := range domain.AgentCommands {
		if c == req.Command {
			known = true
			break
		}
	}
	if !known {
		return nil, domain.Ef(domain.CodeValidationFailed, "unknown agent command %q", req.Command)
	}
	c, err := s.liveClient(ctx, id)
	if err != nil {
		return nil, err
	}
	res, err := c.Command(ctx, req)
	if err != nil {
		return nil, domain.Wrap(domain.CodeUpstreamError, "agent command", err)
	}
	return res, nil
}

// liveClient resolves the loopback client for a running, agent-installed
// instance, or a conflict error explaining why it is unavailable.
func (s *Service) liveClient(ctx context.Context, id string) (*Client, error) {
	in, err := s.inst.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	paths := s.inst.Paths(id)
	if !Installed(paths) {
		return nil, domain.E(domain.CodeConflict, "the agent plugin is not installed on this instance")
	}
	running := in.Status.State == domain.StateRunning || in.Status.State == domain.StateStarting
	if !running {
		return nil, domain.E(domain.CodeConflict, "the instance is not running")
	}
	return s.client(paths, in.Config.Port)
}

// Catalog returns the command pickers (global keys and events) for id's agent.
func (s *Service) Catalog(ctx context.Context, id string) (*domain.AgentCatalog, error) {
	c, err := s.liveClient(ctx, id)
	if err != nil {
		return nil, err
	}
	cat, err := c.Catalog(ctx)
	if err != nil {
		return nil, domain.Wrap(domain.CodeUpstreamError, "agent catalog", err)
	}
	return cat, nil
}

// Chat returns recent chat from id's agent after seq, up to limit lines.
func (s *Service) Chat(ctx context.Context, id string, since int64, limit int) (*domain.AgentChat, error) {
	c, err := s.liveClient(ctx, id)
	if err != nil {
		return nil, err
	}
	ch, err := c.Chat(ctx, since, limit)
	if err != nil {
		return nil, domain.Wrap(domain.CodeUpstreamError, "agent chat", err)
	}
	return ch, nil
}
