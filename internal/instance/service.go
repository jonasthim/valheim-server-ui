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
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/config"
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
}

// New constructs the instance service. bus and sup may be nil-safe fakes in
// tests; in production they are *events.Bus and the configured Supervisor.
func New(db *sql.DB, bus domain.Publisher, sup supervisor.Supervisor, cfg config.Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: db, bus: bus, sup: sup, cfg: cfg, log: log}
}

// Paths returns the canonical directory layout for id.
func (s *Service) Paths(id string) domain.InstancePaths {
	return domain.PathsFor(s.cfg.InstancesDir(), id)
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
		return domain.Validation([]domain.FieldError{{Field: "name", Message: "must be 1-64 characters"}})
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
	r, err := s.getRow(ctx, id)
	if err != nil {
		return nil, err
	}
	paths := s.Paths(id)
	if err := renderLaunch(paths, id, r.Config); err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "write launch.json", err)
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

	st := &domain.InstanceStatus{
		InstanceID:       r.ID,
		State:            state,
		PID:              supSt.PID,
		Since:            since,
		Autostart:        r.Autostart,
		PendingRestart:   r.PendingRestart,
		Detail:           supSt.Detail,
		InstalledBuildID: r.InstalledBuildID,
		BepInExInstalled: fileExists(paths.BepInExDir() + "/core/BepInEx.Preloader.dll"),
		BepInExEnabled:   r.Config.BepInExEnabled,
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

// Poll periodically refreshes every instance's supervisor status and
// publishes instance.status whenever the coarse state changes, so clients
// relying only on SSE (no direct GET) still see systemd-driven transitions
// (crashes, external restarts) and direct-supervisor process exits.
func (s *Service) Poll(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	last := map[string]domain.InstanceState{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollOnce(ctx, last)
		}
	}
}

func (s *Service) pollOnce(ctx context.Context, last map[string]domain.InstanceState) {
	rows, err := s.listRows(ctx)
	if err != nil {
		s.log.Warn("poll: list instances", "err", err)
		return
	}
	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		seen[r.ID] = true
		st, err := s.composeStatus(ctx, r)
		if err != nil {
			s.log.Warn("poll: compose status", "instance", r.ID, "err", err)
			continue
		}
		if prev, ok := last[r.ID]; !ok || prev != st.State {
			last[r.ID] = st.State
			if s.bus != nil {
				s.bus.Publish(domain.Event{Name: domain.EventInstanceStatus, InstanceID: r.ID, Data: st})
			}
		}
	}
	for id := range last {
		if !seen[id] {
			delete(last, id)
		}
	}
}
