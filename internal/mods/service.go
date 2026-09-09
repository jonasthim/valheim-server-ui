package mods

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// InstanceAccessor is the subset of *instance.Service the mods package needs.
// Declared here (rather than depending on the concrete type) so tests can
// substitute a fake and so this package need not import internal/instance
// for anything beyond what it actually calls.
type InstanceAccessor interface {
	Get(ctx context.Context, id string) (*domain.Instance, error)
	Status(ctx context.Context, id string) (*domain.InstanceStatus, error)
	Start(ctx context.Context, id string) (*domain.InstanceStatus, error)
	Stop(ctx context.Context, id string) (*domain.InstanceStatus, error)
	Paths(id string) domain.InstancePaths
	Update(ctx context.Context, id string, name *string, cfg *domain.InstanceConfig, autostart *bool) (*domain.Instance, error)
	MarkPendingRestart(ctx context.Context, id string) error
	PublishStatus(ctx context.Context, id string) error
	Exists(ctx context.Context, id string) (bool, error)
}

// Service satisfies api.ModService and ThunderstoreService satisfies
// api.ThunderstoreService structurally (verified in cmd/valheim-ui/wire_mods.go,
// where they are assigned to deps.Mods / deps.Thunderstore). This package
// deliberately does not import internal/api itself: internal/api's own
// in-package tests import internal/mods (for a real Service in handler
// tests), and this package importing internal/api back would be an import
// cycle for those test files (see internal/jobs/runner.go for the same
// reasoning).

// Service implements api.ModService: BepInEx, installed mods and the config
// editor for one instance.
type Service struct {
	db       *sql.DB
	ts       *Thunderstore
	inst     InstanceAccessor
	runner   *jobs.Runner
	cacheDir string // <data>/cache, for staging uploaded files
	log      *slog.Logger
}

// NewService constructs the mods service.
func NewService(db *sql.DB, ts *Thunderstore, inst InstanceAccessor, runner *jobs.Runner, cacheDir string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: db, ts: ts, inst: inst, runner: runner, cacheDir: cacheDir, log: log}
}

func lowerFullName(owner, name string) string { return strings.ToLower(owner + "-" + name) }

func (s *Service) isRunning(ctx context.Context, instanceID string) (bool, error) {
	st, err := s.inst.Status(ctx, instanceID)
	if err != nil {
		return false, err
	}
	return st.State == domain.StateRunning || st.State == domain.StateStarting, nil
}

// ---------------------------------------------------------------- overview

func (s *Service) Overview(ctx context.Context, instanceID string) (*domain.ModsOverview, error) {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	rows, err := s.listModRows(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	mods := make([]domain.Mod, 0, len(rows))
	for _, r := range rows {
		latest := ""
		if r.Source == domain.ModSourceThunderstore {
			if v, ok := s.ts.LatestVersion(r.Owner, r.Name); ok {
				latest = v
			}
		}
		mods = append(mods, r.toDomain(latest))
	}
	return &domain.ModsOverview{
		BepInEx:        bepinexStatus(inst.Paths, inst.Config.BepInExEnabled, s.ts),
		Mods:           mods,
		PendingRestart: inst.Status.PendingRestart,
	}, nil
}

// ---------------------------------------------------------------- bepinex

func (s *Service) installBepInEx(ctx context.Context, log *jobs.Logger, instanceID string) error {
	paths := s.inst.Paths(instanceID)
	version, ok := s.ts.LatestVersion(domain.BepInExOwner, domain.BepInExName)
	if !ok {
		return domain.E(domain.CodePackageNotFound, "bepinex package not found in the thunderstore index")
	}
	log.Printf("installing BepInEx %s", version)
	zipPath, err := s.ts.Download(ctx, domain.BepInExOwner, domain.BepInExName, version)
	if err != nil {
		return err
	}
	if err := extractBepInExPack(zipPath, paths.Server); err != nil {
		return err
	}
	if err := writePackInfo(paths, packInfo{
		Owner: domain.BepInExOwner, Name: domain.BepInExName, Version: version, InstalledAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	log.Printf("BepInEx %s installed", version)
	return nil
}

func (s *Service) EnqueueBepInExInstall(ctx context.Context, instanceID string, stopIfRunning bool, requestedBy string) (*domain.Job, error) {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	running := inst.Status.State == domain.StateRunning || inst.Status.State == domain.StateStarting
	if running && !stopIfRunning {
		return nil, domain.Ef(domain.CodeInstanceRunning, "instance %q must be stopped first, or pass stop_if_running", instanceID)
	}

	return s.runner.Enqueue(ctx, jobs.Spec{
		Type: domain.JobBepInExInstall, InstanceID: instanceID, Title: "Install/upgrade BepInEx",
		RequestedBy: requestedBy, Exclusive: true,
	}, func(ctx context.Context, log *jobs.Logger) error {
		err := withStoppedInstance(ctx, s.inst, instanceID, log, func(ctx context.Context) error {
			return s.installBepInEx(ctx, log, instanceID)
		})
		if pubErr := s.inst.PublishStatus(ctx, instanceID); pubErr != nil {
			s.log.Warn("publish status after bepinex install", "instance", instanceID, "err", pubErr)
		}
		return err
	})
}

func (s *Service) SetBepInExEnabled(ctx context.Context, instanceID string, enabled bool) (*domain.ModsOverview, error) {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if enabled && !bepinexInstalled(inst.Paths) {
		return nil, domain.E(domain.CodeBepInExMissing, "bepinex is not installed")
	}
	cfg := inst.Config
	cfg.BepInExEnabled = enabled
	if _, err := s.inst.Update(ctx, instanceID, nil, &cfg, nil); err != nil {
		return nil, err
	}
	return s.Overview(ctx, instanceID)
}

// ---------------------------------------------------------------- install / resolver

func (s *Service) installedIndex(ctx context.Context, instanceID string, excludeID int64) (map[string]domain.Mod, error) {
	rows, err := s.listModRows(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]domain.Mod, len(rows))
	for _, r := range rows {
		if r.ID == excludeID {
			continue
		}
		out[r.fullNameLower()] = r.toDomain("")
	}
	return out, nil
}

func logPlan(log *jobs.Logger, needsBepInEx, bepinexAlready bool, plan []PlanStep) {
	log.Printf("dependency plan:")
	if needsBepInEx && !bepinexAlready {
		log.Printf("  - install BepInEx (missing)")
	}
	for _, st := range plan {
		switch {
		case st.AlreadyInstalled:
			log.Printf("  - %s-%s@%s (already installed, skipping)", st.Owner, st.Name, st.Version)
		case st.Upgrade:
			log.Printf("  - %s-%s: upgrade %s -> %s", st.Owner, st.Name, st.FromVersion, st.Version)
		default:
			log.Printf("  - %s-%s@%s (new install)", st.Owner, st.Name, st.Version)
		}
	}
}

// installStep downloads and extracts one plan step, inserting or updating its
// mods row. force makes it (re)install even when st.AlreadyInstalled (used by
// an explicit "update to this version" request).
func (s *Service) installStep(ctx context.Context, log *jobs.Logger, instanceID string, st PlanStep, force bool) error {
	if st.AlreadyInstalled && !force {
		return nil
	}
	paths := s.inst.Paths(instanceID)

	existing, err := s.getModByFullName(ctx, instanceID, st.Owner, st.Name)
	if err != nil {
		return err
	}
	if existing != nil && (st.Upgrade || force) {
		if err := removeManagedFiles(paths.Server, existing.Files); err != nil {
			return fmt.Errorf("remove previous version files: %w", err)
		}
	}

	log.Printf("downloading %s-%s@%s", st.Owner, st.Name, st.Version)
	zipPath, err := s.ts.Download(ctx, st.Owner, st.Name, st.Version)
	if err != nil {
		return err
	}
	files, err := extractPackage(zipPath, paths.Server, st.Owner, st.Name)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if existing != nil {
		if !existing.Enabled {
			if files, err = setFilesEnabled(paths.Server, files, false); err != nil {
				return err
			}
		}
		row := *existing
		row.Version = st.Version
		row.Files = files
		row.Deps = st.Dependencies
		row.WebsiteURL = st.WebsiteURL
		row.IconURL = st.IconURL
		row.UpdatedAt = &now
		return s.saveModRow(ctx, row)
	}

	row := modRow{
		InstanceID: instanceID, Source: domain.ModSourceThunderstore,
		Owner: st.Owner, Name: st.Name, Version: st.Version, Enabled: true,
		Files: files, Deps: st.Dependencies, WebsiteURL: st.WebsiteURL, IconURL: st.IconURL,
		InstalledAt: now,
	}
	_, err = s.insertModRow(ctx, row)
	return err
}

func (s *Service) finishModJob(ctx context.Context, instanceID string) error {
	running, err := s.isRunning(ctx, instanceID)
	if err != nil {
		return err
	}
	if running {
		if err := s.inst.MarkPendingRestart(ctx, instanceID); err != nil {
			return err
		}
	}
	return s.inst.PublishStatus(ctx, instanceID)
}

func (s *Service) runInstall(ctx context.Context, log *jobs.Logger, instanceID, owner, name, version string) error {
	paths := s.inst.Paths(instanceID)
	installed, err := s.installedIndex(ctx, instanceID, 0)
	if err != nil {
		return err
	}
	plan, needsBepInEx, err := resolvePlan(ctx, s.ts, installed, owner, name, version)
	if err != nil {
		return err
	}
	logPlan(log, needsBepInEx, bepinexInstalled(paths), plan)

	if needsBepInEx && !bepinexInstalled(paths) {
		if err := s.installBepInEx(ctx, log, instanceID); err != nil {
			return fmt.Errorf("install bepinex: %w", err)
		}
	}
	for _, st := range plan {
		if err := s.installStep(ctx, log, instanceID, st, false); err != nil {
			return err
		}
	}
	log.SetSummary("plan", planSummary(plan))
	return s.finishModJob(ctx, instanceID)
}

func planSummary(plan []PlanStep) []string {
	out := make([]string, len(plan))
	for i, st := range plan {
		out[i] = st.FullName() + "@" + st.Version
	}
	return out
}

func (s *Service) EnqueueInstall(ctx context.Context, instanceID, owner, name, version, requestedBy string) (*domain.Job, error) {
	if _, err := s.inst.Get(ctx, instanceID); err != nil {
		return nil, err
	}
	if err := validateSlug(owner); err != nil {
		return nil, err
	}
	if err := validateSlug(name); err != nil {
		return nil, err
	}
	if _, err := s.ts.Package(ctx, owner, name); err != nil {
		return nil, err
	}

	title := fmt.Sprintf("Install %s-%s", owner, name)
	return s.runner.Enqueue(ctx, jobs.Spec{Type: domain.JobModInstall, InstanceID: instanceID, Title: title, RequestedBy: requestedBy},
		func(ctx context.Context, log *jobs.Logger) error {
			return s.runInstall(ctx, log, instanceID, owner, name, version)
		})
}

// ---------------------------------------------------------------- upload

func (s *Service) EnqueueUpload(ctx context.Context, instanceID, filename string, r io.Reader, requestedBy string) (*domain.Job, error) {
	if _, err := s.inst.Get(ctx, instanceID); err != nil {
		return nil, err
	}
	lower := strings.ToLower(filename)
	isZip := strings.HasSuffix(lower, ".zip")
	isDLL := strings.HasSuffix(lower, ".dll")
	if !isZip && !isDLL {
		return nil, domain.E(domain.CodeValidationFailed, "upload must be a .zip or .dll file")
	}

	tmpDir := filepath.Join(s.cacheDir, "mods-uploads")
	if err := os.MkdirAll(tmpDir, 0o750); err != nil {
		return nil, fmt.Errorf("create upload temp dir: %w", err)
	}
	tmp, err := os.CreateTemp(tmpDir, "upload-*.bin")
	if err != nil {
		return nil, fmt.Errorf("create temp upload file: %w", err)
	}
	tmpPath := tmp.Name()
	_, copyErr := io.Copy(tmp, r)
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return nil, domain.Wrap(domain.CodeValidationFailed, "read uploaded file", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("close temp upload file: %w", closeErr)
	}
	if isZip {
		if err := verifyZip(tmpPath); err != nil {
			_ = os.Remove(tmpPath)
			return nil, domain.Wrap(domain.CodeValidationFailed, "uploaded file is not a valid zip", err)
		}
	}

	title := "Install uploaded mod " + filename
	return s.runner.Enqueue(ctx, jobs.Spec{Type: domain.JobModInstall, InstanceID: instanceID, Title: title, RequestedBy: requestedBy},
		func(ctx context.Context, log *jobs.Logger) error {
			defer func() { _ = os.Remove(tmpPath) }()
			return s.runUpload(ctx, log, instanceID, tmpPath, filename, isZip)
		})
}

// filesToRemove returns the entries of oldFiles not present in newFiles.
func filesToRemove(oldFiles, newFiles []string) []string {
	newSet := make(map[string]bool, len(newFiles))
	for _, f := range newFiles {
		newSet[f] = true
	}
	var out []string
	for _, f := range oldFiles {
		if !newSet[f] {
			out = append(out, f)
		}
	}
	return out
}

func (s *Service) runUpload(ctx context.Context, log *jobs.Logger, instanceID, tmpPath, filename string, isZip bool) error {
	paths := s.inst.Paths(instanceID)

	var owner, name, version string
	var deps []string
	if isZip {
		var err error
		owner, name, version, deps, err = manualZipIdentity(tmpPath)
		if err != nil {
			return err
		}
	} else {
		owner = "local"
		name = sanitizeSlug(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
		version = "0.0.0"
	}
	if err := validateSlug(owner); err != nil {
		return err
	}
	if err := validateSlug(name); err != nil {
		return err
	}

	existing, err := s.getModByFullName(ctx, instanceID, owner, name)
	if err != nil {
		return err
	}

	var files []string
	if isZip {
		log.Printf("installing uploaded package %s as %s-%s@%s", filename, owner, name, version)
		files, err = extractPackage(tmpPath, paths.Server, owner, name)
	} else {
		log.Printf("installing uploaded dll %s as %s-%s", filename, owner, name)
		_, _, files, err = installSingleDLL(tmpPath, paths.Server, filename)
	}
	if err != nil {
		return err
	}

	if existing != nil {
		if stale := filesToRemove(existing.Files, files); len(stale) > 0 {
			if err := removeManagedFiles(paths.Server, stale); err != nil {
				return err
			}
		}
	}

	now := time.Now().UTC()
	if existing != nil {
		if !existing.Enabled {
			if files, err = setFilesEnabled(paths.Server, files, false); err != nil {
				return err
			}
		}
		row := *existing
		row.Version = version
		row.Files = files
		row.Deps = deps
		row.UpdatedAt = &now
		if err := s.saveModRow(ctx, row); err != nil {
			return err
		}
	} else {
		row := modRow{
			InstanceID: instanceID, Source: domain.ModSourceManual,
			Owner: owner, Name: name, Version: version, Enabled: true,
			Files: files, Deps: deps, InstalledAt: now,
		}
		if _, err := s.insertModRow(ctx, row); err != nil {
			return err
		}
	}
	log.Printf("installed %s-%s@%s", owner, name, version)
	return s.finishModJob(ctx, instanceID)
}

// ---------------------------------------------------------------- enable/disable/uninstall/update

func (s *Service) SetEnabled(ctx context.Context, instanceID string, modID int64, enabled bool) (*domain.Mod, error) {
	row, err := s.getModRow(ctx, instanceID, modID)
	if err != nil {
		return nil, err
	}
	paths := s.inst.Paths(instanceID)

	if row.Enabled != enabled {
		files, err := setFilesEnabled(paths.Server, row.Files, enabled)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		row.Files = files
		row.Enabled = enabled
		row.UpdatedAt = &now
		if err := s.saveModRow(ctx, row); err != nil {
			return nil, err
		}
		if err := s.finishModJob(ctx, instanceID); err != nil {
			return nil, err
		}
	}

	latest := ""
	if row.Source == domain.ModSourceThunderstore {
		if v, ok := s.ts.LatestVersion(row.Owner, row.Name); ok {
			latest = v
		}
	}
	m := row.toDomain(latest)
	return &m, nil
}

func (s *Service) EnqueueUninstall(ctx context.Context, instanceID string, modID int64, requestedBy string) (*domain.Job, error) {
	row, err := s.getModRow(ctx, instanceID, modID)
	if err != nil {
		return nil, err
	}
	title := fmt.Sprintf("Uninstall %s-%s", row.Owner, row.Name)
	return s.runner.Enqueue(ctx, jobs.Spec{Type: domain.JobModUninstall, InstanceID: instanceID, Title: title, RequestedBy: requestedBy},
		func(ctx context.Context, log *jobs.Logger) error {
			paths := s.inst.Paths(instanceID)
			log.Printf("removing %s-%s@%s", row.Owner, row.Name, row.Version)
			if err := removeManagedFiles(paths.Server, row.Files); err != nil {
				return err
			}
			if err := s.deleteModRow(ctx, instanceID, modID); err != nil {
				return err
			}
			return s.finishModJob(ctx, instanceID)
		})
}

func (s *Service) runUpdate(ctx context.Context, log *jobs.Logger, instanceID string, row modRow, version string) error {
	installed, err := s.installedIndex(ctx, instanceID, row.ID)
	if err != nil {
		return err
	}
	plan, needsBepInEx, err := resolvePlan(ctx, s.ts, installed, row.Owner, row.Name, version)
	if err != nil {
		return err
	}
	paths := s.inst.Paths(instanceID)
	logPlan(log, needsBepInEx, bepinexInstalled(paths), plan)

	if needsBepInEx && !bepinexInstalled(paths) {
		if err := s.installBepInEx(ctx, log, instanceID); err != nil {
			return fmt.Errorf("install bepinex: %w", err)
		}
	}
	for _, st := range plan {
		force := strings.EqualFold(st.Owner, row.Owner) && strings.EqualFold(st.Name, row.Name)
		if err := s.installStep(ctx, log, instanceID, st, force); err != nil {
			return err
		}
	}
	log.SetSummary("plan", planSummary(plan))
	return s.finishModJob(ctx, instanceID)
}

func (s *Service) EnqueueUpdate(ctx context.Context, instanceID string, modID int64, version, requestedBy string) (*domain.Job, error) {
	row, err := s.getModRow(ctx, instanceID, modID)
	if err != nil {
		return nil, err
	}
	if row.Source != domain.ModSourceThunderstore {
		return nil, domain.E(domain.CodeValidationFailed, "only thunderstore mods can be updated")
	}
	title := fmt.Sprintf("Update %s-%s", row.Owner, row.Name)
	return s.runner.Enqueue(ctx, jobs.Spec{Type: domain.JobModUpdate, InstanceID: instanceID, Title: title, RequestedBy: requestedBy},
		func(ctx context.Context, log *jobs.Logger) error {
			return s.runUpdate(ctx, log, instanceID, row, version)
		})
}

// ---------------------------------------------------------------- config editor

func (s *Service) ListConfigs(ctx context.Context, instanceID string) ([]domain.ConfigFileInfo, error) {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return listConfigFiles(inst.Paths)
}

func (s *Service) GetConfig(ctx context.Context, instanceID, file string) (*domain.ConfigFile, error) {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return readConfigFile(inst.Paths, file)
}

func (s *Service) UpdateConfig(ctx context.Context, instanceID, file string, upd domain.ConfigFileUpdate) (*domain.ConfigFile, error) {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	cf, err := writeConfigFile(inst.Paths, file, upd)
	if err != nil {
		return nil, err
	}
	if err := s.finishModJob(ctx, instanceID); err != nil {
		return nil, err
	}
	return cf, nil
}

// ---------------------------------------------------------------- thunderstore

// ThunderstoreService implements api.ThunderstoreService on top of a
// *Thunderstore client, adding the job-queued index refresh.
type ThunderstoreService struct {
	ts     *Thunderstore
	runner *jobs.Runner
}

// NewThunderstoreService constructs the service.
func NewThunderstoreService(ts *Thunderstore, runner *jobs.Runner) *ThunderstoreService {
	return &ThunderstoreService{ts: ts, runner: runner}
}

func (s *ThunderstoreService) Search(ctx context.Context, q domain.PackageSearch) (*domain.PackageSearchResult, error) {
	return s.ts.Search(ctx, q)
}

func (s *ThunderstoreService) Package(ctx context.Context, owner, name string) (*domain.Package, error) {
	return s.ts.Package(ctx, owner, name)
}

func (s *ThunderstoreService) Categories(ctx context.Context) ([]string, error) {
	return s.ts.Categories(ctx)
}

func (s *ThunderstoreService) EnqueueRefresh(ctx context.Context, requestedBy string) (*domain.Job, error) {
	return s.runner.Enqueue(ctx, jobs.Spec{
		Type: domain.JobThunderstoreRefresh, InstanceID: "", Title: "Refresh Thunderstore index",
		RequestedBy: requestedBy, Exclusive: true,
	}, func(ctx context.Context, log *jobs.Logger) error {
		log.Printf("refreshing thunderstore index")
		if err := s.ts.Refresh(ctx); err != nil {
			return err
		}
		updatedAt := s.ts.IndexUpdatedAt()
		log.Printf("index refreshed (updated_at=%s)", updatedAt.Format(time.RFC3339))
		log.SetSummary("updated_at", updatedAt)
		return nil
	})
}
