// Package instance implements api.InstanceService: instance CRUD, directory
// tree management, launch.json rendering, supervisor orchestration and status
// composition. See docs/ARCHITECTURE.md §3, §5.1, §6, §7, §8.
package instance

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// pollInterval is how often Poll checks every instance's supervisor status.
const pollInterval = 5 * time.Second

// maxTailLines caps GET /logs?lines (openapi.yaml).
const maxTailLines = 5000

// defaultTailLines is used when lines is omitted or <= 0.
const defaultTailLines = 500

// Service implements api.InstanceService on top of the instances table.
type Service struct {
	db  *sql.DB
	bus domain.Publisher
	sup supervisor.Supervisor
	cfg config.Config
	log *slog.Logger

	mu        sync.RWMutex
	enrichers []domain.StatusEnricher
	// preStart hooks run after launch.json is rendered and before the
	// supervisor starts the process (the agent writes its plugin config here).
	preStart []PreStartHook

	// events records the per-instance lifecycle timeline (F-1.2: crash
	// detection). Built from the same *sql.DB as the instances table.
	events *db.InstanceEventRepo
	// expected marks instance ids with a manager-initiated Start/Stop/Restart
	// in flight (set by MarkExpectedTransition), so the poll loop's crash
	// detection can tell "we did this" from "this crashed and something else
	// (systemd's Restart= policy, an operator running the binary by hand)
	// brought it back".
	expected map[string]time.Time
	// crashCache holds CrashCount24h/LastCrashAt/LastExitDetail per instance,
	// refreshed once per poll cycle (refreshCrashCache) so composeStatus
	// (also called per HTTP request) never queries instance_events directly.
	crashCache map[string]crashSummary
}

// New constructs the instance service. bus and sup may be nil-safe fakes in
// tests; in production they are *events.Bus and the configured Supervisor.
func New(sqldb *sql.DB, bus domain.Publisher, sup supervisor.Supervisor, cfg config.Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: sqldb, bus: bus, sup: sup, cfg: cfg, log: log, events: db.NewInstanceEventRepo(sqldb)}
}

// Paths returns the canonical directory layout for id.
func (s *Service) Paths(id string) domain.InstancePaths {
	return domain.PathsFor(s.cfg.InstancesDir(), id)
}

// PreStartHook prepares an instance's files right before it starts.
type PreStartHook func(ctx context.Context, id string, paths domain.InstancePaths, cfg domain.InstanceConfig) error

// RegisterPreStart adds a hook run by Start and Restart after launch.json is
// written. A failing hook aborts the start.
func (s *Service) RegisterPreStart(h PreStartHook) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preStart = append(s.preStart, h)
}

func (s *Service) runPreStart(ctx context.Context, id string, paths domain.InstancePaths, cfg domain.InstanceConfig) error {
	s.mu.RLock()
	hooks := append([]PreStartHook(nil), s.preStart...)
	s.mu.RUnlock()
	for _, h := range hooks {
		if err := h(ctx, id, paths, cfg); err != nil {
			return domain.Wrap(domain.CodeInternal, "prepare start", err)
		}
	}
	return nil
}

// RegisterEnricher adds a StatusEnricher run on every composed InstanceStatus.
// Called by other feature packages during wiring (players/a2s, update
// checks, bepinex). Safe to call concurrently with Status/Poll.
func (s *Service) RegisterEnricher(e domain.StatusEnricher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enrichers = append(s.enrichers, e)
}

func (s *Service) enrichersSnapshot() []domain.StatusEnricher {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.StatusEnricher, len(s.enrichers))
	copy(out, s.enrichers)
	return out
}

// expectedWindow is how long after MarkExpectedTransition the poll loop
// treats a state/restart change it observes for that instance as expected
// (manager-initiated) rather than a crash.
const expectedWindow = 3 * time.Minute

// MarkExpectedTransition records that id is about to go through a
// manager-initiated state change (Start/Stop/Restart, or the update job's
// stop/start around a game-file update), so the poll loop's crash detection
// does not mistake the resulting transition for a crash.
func (s *Service) MarkExpectedTransition(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.expected == nil {
		s.expected = map[string]time.Time{}
	}
	s.expected[id] = time.Now()
}

// recentlyExpected reports whether id had a MarkExpectedTransition call
// within the last expectedWindow.
func (s *Service) recentlyExpected(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.expected[id]
	return ok && time.Since(t) <= expectedWindow
}

// ---------------------------------------------------------------- CRUD

func (s *Service) List(ctx context.Context) ([]domain.Instance, error) {
	rows, err := s.listRows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Instance, 0, len(rows))
	for _, r := range rows {
		inst, err := s.toInstance(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, *inst)
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Instance, error) {
	r, err := s.getRow(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.toInstance(ctx, r)
}

func validateDisplayName(name string) error {
	n := strings.TrimSpace(name)
	if n == "" || len(name) > 64 {
		return domain.Validation([]domain.FieldError{{Field: "display_name", Message: "must be 1-64 characters"}})
	}
	return nil
}

func (s *Service) Create(ctx context.Context, id, name string, cfg domain.InstanceConfig, autostart bool) (*domain.Instance, error) {
	if !domain.InstanceIDPattern.MatchString(id) {
		return nil, domain.Validation([]domain.FieldError{{Field: "id", Message: "must match ^[a-z0-9][a-z0-9-]{0,31}$"}})
	}
	if err := validateDisplayName(name); err != nil {
		return nil, err
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	exists, err := s.existsRow(ctx, id)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, domain.Ef(domain.CodeConflict, "instance %q already exists", id)
	}

	if err := s.checkPortOverlap(ctx, id, cfg.Port); err != nil {
		return nil, err
	}

	paths := s.Paths(id)
	if err := createTree(paths); err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "create instance directories", err)
	}
	if err := renderLaunch(paths, id, cfg); err != nil {
		_ = removeTree(s.cfg.InstancesDir(), paths.Root)
		return nil, domain.Wrap(domain.CodeInternal, "write launch.json", err)
	}

	now := time.Now().UTC()
	r := row{ID: id, Name: name, Config: cfg, Autostart: autostart, CreatedAt: now, UpdatedAt: now}
	if err := s.insertRow(ctx, r); err != nil {
		_ = removeTree(s.cfg.InstancesDir(), paths.Root)
		return nil, domain.Wrap(domain.CodeInternal, "save instance", err)
	}

	return s.toInstance(ctx, r)
}

func (s *Service) checkPortOverlap(ctx context.Context, selfID string, port int) error {
	rows, err := s.listRows(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ID == selfID {
			continue
		}
		if domain.PortsOverlap(port, r.Config.Port) {
			return domain.Ef(domain.CodePortInUse, "port range %d-%d overlaps instance %q (port %d)",
				port, port+domain.PortSpan-1, r.ID, r.Config.Port)
		}
	}
	return nil
}

func (s *Service) Update(ctx context.Context, id string, name *string, cfg *domain.InstanceConfig, autostart *bool) (*domain.Instance, error) {
	r, err := s.getRow(ctx, id)
	if err != nil {
		return nil, err
	}

	configChanged := false
	if name != nil {
		if err := validateDisplayName(*name); err != nil {
			return nil, err
		}
		r.Name = *name
	}
	if cfg != nil {
		newCfg := *cfg
		newCfg.ApplyDefaults()
		if err := newCfg.Validate(); err != nil {
			return nil, err
		}
		if err := s.checkPortOverlap(ctx, id, newCfg.Port); err != nil {
			return nil, err
		}
		configChanged = !reflect.DeepEqual(newCfg, r.Config)
		r.Config = newCfg
	}

	autostartChanged := false
	if autostart != nil && *autostart != r.Autostart {
		autostartChanged = true
		r.Autostart = *autostart
	}

	if configChanged {
		st, err := s.sup.Status(ctx, id)
		if err != nil {
			return nil, domain.Wrap(domain.CodeInternal, "check instance status", err)
		}
		if st.State == supervisor.StateRunning || st.State == supervisor.StateStarting {
			r.PendingRestart = true
		}
	}

	paths := s.Paths(id)
	if err := renderLaunch(paths, id, r.Config); err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "write launch.json", err)
	}

	r.UpdatedAt = time.Now().UTC()
	if err := s.saveRow(ctx, r); err != nil {
		return nil, err
	}

	if autostartChanged {
		if err := s.sup.SetAutostart(ctx, id, r.Autostart); err != nil {
			return nil, err
		}
	}

	return s.toInstance(ctx, r)
}

func (s *Service) Delete(ctx context.Context, id string, deleteFiles bool) error {
	if _, err := s.getRow(ctx, id); err != nil {
		return err
	}
	st, err := s.sup.Status(ctx, id)
	if err != nil {
		return domain.Wrap(domain.CodeInternal, "check instance status", err)
	}
	switch st.State {
	case supervisor.StateRunning, supervisor.StateStarting, supervisor.StateStopping:
		return domain.Ef(domain.CodeInstanceRunning, "instance %q must be stopped before deletion", id)
	}

	if err := s.deleteRow(ctx, id); err != nil {
		return err
	}
	if deleteFiles {
		if err := removeTree(s.cfg.InstancesDir(), s.Paths(id).Root); err != nil {
			return domain.Wrap(domain.CodeInternal, "delete instance files", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------- lifecycle

func (s *Service) Start(ctx context.Context, id string) (*domain.InstanceStatus, error) {
	s.MarkExpectedTransition(id)
	r, err := s.getRow(ctx, id)
	if err != nil {
		return nil, err
	}
	paths := s.Paths(id)
	if !fileExists(paths.ServerBinary()) {
		return nil, domain.Ef(domain.CodeInstanceNotInstalled, "instance %q is not installed", id)
	}
	if err := renderLaunch(paths, id, r.Config); err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "write launch.json", err)
	}
	if err := s.runPreStart(ctx, id, paths, r.Config); err != nil {
		return nil, err
	}
	if err := rotateConsoleLog(paths); err != nil {
		s.log.Warn("rotate console.log failed", "instance", id, "err", err)
	}
	if err := s.sup.Start(ctx, id); err != nil {
		return nil, err
	}

	r.PendingRestart = false
	r.UpdatedAt = time.Now().UTC()
	if err := s.saveRow(ctx, r); err != nil {
		return nil, err
	}
	return s.publishStatus(ctx, r)
}

func (s *Service) Stop(ctx context.Context, id string) (*domain.InstanceStatus, error) {
	s.MarkExpectedTransition(id)
	r, err := s.getRow(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.sup.Stop(ctx, id); err != nil {
		return nil, err
	}
	return s.publishStatus(ctx, r)
}

func (s *Service) Restart(ctx context.Context, id string) (*domain.InstanceStatus, error) {
	s.MarkExpectedTransition(id)
	r, err := s.getRow(ctx, id)
	if err != nil {
		return nil, err
	}
	paths := s.Paths(id)
	if err := renderLaunch(paths, id, r.Config); err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "write launch.json", err)
	}
	if err := s.runPreStart(ctx, id, paths, r.Config); err != nil {
		return nil, err
	}
	if err := s.sup.Restart(ctx, id); err != nil {
		return nil, err
	}

	r.PendingRestart = false
	r.UpdatedAt = time.Now().UTC()
	if err := s.saveRow(ctx, r); err != nil {
		return nil, err
	}
	return s.publishStatus(ctx, r)
}

func (s *Service) Status(ctx context.Context, id string) (*domain.InstanceStatus, error) {
	r, err := s.getRow(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.composeStatus(ctx, r)
}

// publishStatus composes the current status and publishes instance.status.
func (s *Service) publishStatus(ctx context.Context, r row) (*domain.InstanceStatus, error) {
	st, err := s.composeStatus(ctx, r)
	if err != nil {
		return nil, err
	}
	if s.bus != nil {
		s.bus.Publish(domain.Event{Name: domain.EventInstanceStatus, InstanceID: r.ID, Data: st})
	}
	return st, nil
}

// PublishStatus composes and publishes id's current status. Other packages
// (jobs, mods, backups) call this after actions that change what Status()
// would report, so the UI updates without waiting for the next poll tick.
func (s *Service) PublishStatus(ctx context.Context, id string) error {
	r, err := s.getRow(ctx, id)
	if err != nil {
		return err
	}
	_, err = s.publishStatus(ctx, r)
	return err
}

func (s *Service) composeStatus(ctx context.Context, r row) (*domain.InstanceStatus, error) {
	supSt, err := s.sup.Status(ctx, r.ID)
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "read instance status", err)
	}
	paths := s.Paths(r.ID)
	installed := fileExists(paths.ServerBinary())
	state := domain.InstanceState(supSt.State)
	if !installed && state == domain.StateStopped {
		state = domain.StateNotInstalled
	}

	var since *time.Time
	if !supSt.Since.IsZero() {
		t := supSt.Since
		since = &t
	}

	cs := s.crashSummaryFor(r.ID)

	st := &domain.InstanceStatus{
		InstanceID:       r.ID,
		State:            state,
		PID:              supSt.PID,
		Since:            since,
		Autostart:        r.Autostart,
		PendingRestart:   r.PendingRestart,
		Detail:           supSt.Detail,
		InstalledBuildID: r.InstalledBuildID,
		UpdateAvailable:  r.LatestBuildID != "" && r.InstalledBuildID != "" && r.LatestBuildID != r.InstalledBuildID,
		BepInExInstalled: fileExists(filepath.Join(paths.BepInExDir(), "core", "BepInEx.Preloader.dll")),
		BepInExEnabled:   r.Config.BepInExEnabled,
		CrashCount24h:    cs.count24h,
		LastCrashAt:      cs.lastAt,
		LastExitDetail:   cs.detail,
	}
	for _, e := range s.enrichersSnapshot() {
		e.Enrich(ctx, st)
	}
	return st, nil
}

func (s *Service) toInstance(ctx context.Context, r row) (*domain.Instance, error) {
	st, err := s.composeStatus(ctx, r)
	if err != nil {
		return nil, err
	}
	return &domain.Instance{
		ID:               r.ID,
		Name:             r.Name,
		Config:           r.Config,
		Autostart:        r.Autostart,
		PendingRestart:   r.PendingRestart,
		InstalledBuildID: r.InstalledBuildID,
		LatestBuildID:    r.LatestBuildID,
		BuildIDCheckedAt: r.BuildIDCheckedAt,
		Status:           *st,
		Paths:            s.Paths(r.ID),
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}, nil
}

// ---------------------------------------------------------------- logs

func (s *Service) TailLog(ctx context.Context, id string, lines int) ([]string, error) {
	if _, err := s.getRow(ctx, id); err != nil {
		return nil, err
	}
	if lines <= 0 {
		lines = defaultTailLines
	}
	if lines > maxTailLines {
		lines = maxTailLines
	}
	out, err := tailFile(s.Paths(id).ConsoleLog(), lines)
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "read console.log", err)
	}
	return out, nil
}

func (s *Service) OpenLog(ctx context.Context, id string) (io.ReadCloser, error) {
	if _, err := s.getRow(ctx, id); err != nil {
		return nil, err
	}
	f, err := os.Open(s.Paths(id).ConsoleLog())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.NotFound("log")
		}
		return nil, domain.Wrap(domain.CodeInternal, "open console.log", err)
	}
	return f, nil
}

// defaultSearchLimit/maxSearchLimit bound SearchLogs' limit (openapi.yaml
// GET /logs/search).
const (
	defaultSearchLimit = 500
	maxSearchLimit     = 2000
)

// ListLogFiles lists the console log, rotated console logs, and BepInEx's
// LogOutput.log available for id, each only if present (F-2.6).
func (s *Service) ListLogFiles(ctx context.Context, id string) ([]domain.LogFileInfo, error) {
	if _, err := s.getRow(ctx, id); err != nil {
		return nil, err
	}
	files, err := listLogFiles(s.Paths(id))
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "list log files", err)
	}
	out := make([]domain.LogFileInfo, len(files))
	for i, f := range files {
		out[i] = f.Info
	}
	return out, nil
}

// OpenLogFile opens one log file by its base name, as returned by
// ListLogFiles. name is only ever used as a key into that listing, never
// joined onto a directory: a name that is not a plain basename, or that does
// not exactly match a listed file, is a not-found error before any file is
// opened.
func (s *Service) OpenLogFile(ctx context.Context, id, name string) (io.ReadCloser, error) {
	if _, err := s.getRow(ctx, id); err != nil {
		return nil, err
	}
	if name == "" || filepath.Base(name) != name {
		return nil, domain.NotFound("log file")
	}
	files, err := listLogFiles(s.Paths(id))
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "list log files", err)
	}
	for _, f := range files {
		if f.Info.Name != name {
			continue
		}
		file, err := os.Open(f.Path) //nolint:gosec // f.Path came from listLogFiles, not user input
		if err != nil {
			if os.IsNotExist(err) {
				return nil, domain.NotFound("log file")
			}
			return nil, domain.Wrap(domain.CodeInternal, "open log file", err)
		}
		return file, nil
	}
	return nil, domain.NotFound("log file")
}

// SearchLogs searches every log file available for id, newest match first
// (F-2.6). limit <= 0 defaults to 500 and is capped at 2000.
func (s *Service) SearchLogs(ctx context.Context, id, q string, useRegex bool, limit int) ([]domain.LogMatch, error) {
	if _, err := s.getRow(ctx, id); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}
	files, err := listLogFiles(s.Paths(id))
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "list log files", err)
	}
	return searchLogFiles(files, q, useRegex, limit)
}

// ---------------------------------------------------------------- helpers for other packages

// MarkPendingRestart flags id as needing a restart to apply a change made
// outside the instance service (mods, config file edits). No-op error if id
// does not exist.
func (s *Service) MarkPendingRestart(ctx context.Context, id string) error {
	r, err := s.getRow(ctx, id)
	if err != nil {
		return err
	}
	if r.PendingRestart {
		return nil
	}
	r.PendingRestart = true
	r.UpdatedAt = time.Now().UTC()
	return s.saveRow(ctx, r)
}

// SetInstalledBuildID records the SteamCMD-reported build id after an
// install/update job completes (WP-05).
func (s *Service) SetInstalledBuildID(ctx context.Context, id, buildID string) error {
	r, err := s.getRow(ctx, id)
	if err != nil {
		return err
	}
	r.InstalledBuildID = buildID
	r.UpdatedAt = time.Now().UTC()
	return s.saveRow(ctx, r)
}

// pollState is what Poll remembers about an instance between ticks: the
// composed coarse state (for the existing instance.status-on-change publish)
// and the supervisor's restart counter/start timestamp (for crash detection).
type pollState struct {
	state    domain.InstanceState
	restarts int
	since    time.Time
}

// crashSummary is CrashCount24h/LastCrashAt/LastExitDetail cached once per
// poll cycle by refreshCrashCache, and read by composeStatus.
type crashSummary struct {
	count24h int
	lastAt   *time.Time
	detail   string
}

// Poll periodically refreshes every instance's supervisor status and
// publishes instance.status whenever the coarse state changes, so clients
// relying only on SSE (no direct GET) still see systemd-driven transitions
// (crashes, external restarts) and direct-supervisor process exits. It also
// detects crashes (a restart count increase, or a new process start with no
// matching MarkExpectedTransition), recording an instance_events row and
// publishing instance.crashed for each.
func (s *Service) Poll(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	last := map[string]pollState{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollOnce(ctx, last)
		}
	}
}

func (s *Service) pollOnce(ctx context.Context, last map[string]pollState) {
	rows, err := s.listRows(ctx)
	if err != nil {
		s.log.Warn("poll: list instances", "err", err)
		return
	}
	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		seen[r.ID] = true

		supSt, err := s.sup.Status(ctx, r.ID)
		if err != nil {
			s.log.Warn("poll: read supervisor status", "instance", r.ID, "err", err)
			continue
		}

		prev, hadPrev := last[r.ID]
		crashed := false
		if hadPrev {
			restartsIncreased := supSt.Restarts > prev.restarts
			supState := domain.InstanceState(supSt.State)
			runningish := supState == domain.StateRunning || supState == domain.StateStarting
			// Only "moved forward" if there was already a since to move
			// forward from: the first-ever observed start (prev.since zero)
			// is a start, not a crash.
			sinceMovedForward := !prev.since.IsZero() && !supSt.Since.IsZero() &&
				!supSt.Since.Equal(prev.since) && runningish
			crashed = restartsIncreased || (sinceMovedForward && !s.recentlyExpected(r.ID))
		}
		if crashed {
			s.recordCrash(ctx, r.ID, supSt.ExitDetail)
		}
		// Refresh every tick (not just on a crash) so the 24h window ages out
		// old crashes even when nothing new happens.
		s.refreshCrashCache(ctx, r.ID)

		st, err := s.composeStatus(ctx, r)
		if err != nil {
			s.log.Warn("poll: compose status", "instance", r.ID, "err", err)
			last[r.ID] = pollState{state: domain.InstanceState(supSt.State), restarts: supSt.Restarts, since: supSt.Since}
			continue
		}

		stateChanged := !hadPrev || prev.state != st.State
		if stateChanged && hadPrev {
			s.recordLifecycleTransition(ctx, r.ID, st.State)
		}
		if stateChanged || crashed {
			if s.bus != nil {
				s.bus.Publish(domain.Event{Name: domain.EventInstanceStatus, InstanceID: r.ID, Data: st})
			}
		}

		last[r.ID] = pollState{state: st.State, restarts: supSt.Restarts, since: supSt.Since}
	}
	for id := range last {
		if !seen[id] {
			delete(last, id)
		}
	}
}

// recordCrash inserts a crash row and publishes instance.crashed. A DB
// failure is logged, not fatal to polling (the event just won't show up in
// the timeline; the forced instance.status publish still happens).
func (s *Service) recordCrash(ctx context.Context, id, detail string) {
	ev := domain.InstanceEvent{InstanceID: id, At: time.Now().UTC(), Kind: "crash", Detail: detail}
	if s.events != nil {
		if err := s.events.Insert(ctx, ev); err != nil {
			s.log.Warn("poll: record crash event", "instance", id, "err", err)
		}
	}
	if s.bus != nil {
		s.bus.Publish(domain.Event{Name: domain.EventInstanceCrashed, InstanceID: id, Data: ev})
	}
}

// recordLifecycleTransition inserts a start/stop row when the poll loop
// observes the instance's composed state settle into running or stopped.
// Intermediate states (starting/stopping/failed/not_installed) are not
// recorded; "ready" and "update" events come from elsewhere.
func (s *Service) recordLifecycleTransition(ctx context.Context, id string, state domain.InstanceState) {
	if s.events == nil {
		return
	}
	var kind string
	switch state {
	case domain.StateRunning:
		kind = "start"
	case domain.StateStopped:
		kind = "stop"
	default:
		return
	}
	ev := domain.InstanceEvent{InstanceID: id, At: time.Now().UTC(), Kind: kind}
	if err := s.events.Insert(ctx, ev); err != nil {
		s.log.Warn("poll: record lifecycle event", "instance", id, "kind", kind, "err", err)
	}
}

// refreshCrashCache recomputes id's 24h crash count and last-crash summary
// from instance_events, once per poll cycle, so composeStatus (also called
// per HTTP request via Get/List/Status) never queries the database directly.
func (s *Service) refreshCrashCache(ctx context.Context, id string) {
	if s.events == nil {
		return
	}
	count, err := s.events.CountSince(ctx, id, "crash", time.Now().Add(-24*time.Hour))
	if err != nil {
		s.log.Warn("poll: count crashes", "instance", id, "err", err)
		return
	}
	var lastAt *time.Time
	var detail string
	if last, err := s.events.Latest(ctx, id, "crash"); err != nil {
		s.log.Warn("poll: latest crash", "instance", id, "err", err)
	} else if last != nil {
		t := last.At
		lastAt = &t
		detail = last.Detail
	}

	s.mu.Lock()
	if s.crashCache == nil {
		s.crashCache = map[string]crashSummary{}
	}
	s.crashCache[id] = crashSummary{count24h: count, lastAt: lastAt, detail: detail}
	s.mu.Unlock()
}

func (s *Service) crashSummaryFor(id string) crashSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.crashCache[id]
}

// InstanceEvents returns id's lifecycle timeline, newest first (GET
// /instances/{id}/events).
func (s *Service) InstanceEvents(ctx context.Context, id string, limit int, before *time.Time) ([]domain.InstanceEvent, error) {
	if _, err := s.getRow(ctx, id); err != nil {
		return nil, err
	}
	if s.events == nil {
		return []domain.InstanceEvent{}, nil
	}
	return s.events.List(ctx, id, limit, before)
}

// SetBuildIDs records the result of a Steam update check (installed vs latest
// public build) so Status can report update_available. Used by the update
// checker's StoreFunc.
func (s *Service) SetBuildIDs(ctx context.Context, id, installed, latest string, checkedAt time.Time) error {
	r, err := s.getRow(ctx, id)
	if err != nil {
		return err
	}
	if installed != "" {
		r.InstalledBuildID = installed
	}
	r.LatestBuildID = latest
	t := checkedAt.UTC()
	r.BuildIDCheckedAt = &t
	r.UpdatedAt = time.Now().UTC()
	return s.saveRow(ctx, r)
}

// Exists reports whether an instance row exists.
func (s *Service) Exists(ctx context.Context, id string) (bool, error) {
	_, err := s.getRow(ctx, id)
	if err == nil {
		return true, nil
	}
	if domain.AsError(err).Code == domain.CodeNotFound {
		return false, nil
	}
	return false, err
}
