// Package backup implements api.BackupService: manager-side zip backups with
// retention, restore, and world upload/download/import/delete/export
// (WP-06). See docs/ARCHITECTURE.md §3, §9, §10.
//
// It deliberately does not import internal/api: internal/api's own internal
// test files import internal/backup to build a real Service for handler
// tests, and backup importing api back would be an import cycle for those
// test files (see internal/instance/jobs.go and internal/jobs/runner.go for
// the same pattern). api.BackupService satisfaction is checked structurally
// where it matters -- the assignment in cmd/valheim-ui/wire_backups.go,
// which does import both packages.
package backup

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/backup/remote"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// stopPollInterval/stopPollTimeout bound how long a restore job that stopped
// the instance itself waits for it to actually reach stopped before giving
// up. Mirrors internal/instance/jobs.go's update job (ARCHITECTURE.md §9).
const (
	stopPollInterval = 500 * time.Millisecond
	stopPollTimeout  = 150 * time.Second
)

// Service implements api.BackupService on top of the backups table and each
// instance's backups/ and save/ directories.
type Service struct {
	db         *sql.DB
	inst       *instance.Service
	runner     *jobs.Runner
	bus        domain.Publisher
	appVersion string
	log        *slog.Logger

	// uploader and targetsFn back the off-site copy feature (F-1.4); both are
	// nil until SetUploader/SetTargets are called from wire_backups.go, in
	// which case Create silently skips the off-site copy (there is nothing
	// sane to do without them).
	uploader  remote.Uploader
	targetsFn func(ctx context.Context) ([]domain.BackupTarget, error)
}

// New constructs the backup service.
func New(db *sql.DB, inst *instance.Service, runner *jobs.Runner, bus domain.Publisher, appVersion string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: db, inst: inst, runner: runner, bus: bus, appVersion: appVersion, log: log}
}

// SetUploader configures the off-site uploader used by Create and
// EnqueueRemoteUpload (F-1.4). Called once from cmd/valheim-ui/wire_backups.go.
func (s *Service) SetUploader(u remote.Uploader) { s.uploader = u }

// SetTargets configures how the configured backup targets are read (F-1.4);
// wire_backups.go points it at deps.Settings.Get(ctx).Backups.Targets.
func (s *Service) SetTargets(fn func(ctx context.Context) ([]domain.BackupTarget, error)) {
	s.targetsFn = fn
}

// isBusyState reports whether st means it is not safe to overwrite save
// files in place without stopping the instance first.
func isBusyState(st domain.InstanceState) bool {
	switch st {
	case domain.StateRunning, domain.StateStarting, domain.StateStopping:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------- create

// Create makes a backup of instanceID's active world right now. It is the
// synchronous core used by EnqueueBackup's job, by PreUpdateBackup (WP-05's
// update job), by the restore job's pre_restore step, and directly by the
// scheduler (WP-07).
func (s *Service) Create(ctx context.Context, instanceID string, kind domain.BackupKind, note string) (*domain.Backup, error) {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	paths := s.inst.Paths(instanceID)
	world := inst.Config.World
	worldsDir := paths.WorldsDir()
	save, err := scanWorld(worldsDir, world)
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "scan worlds", err)
	}
	if !save.HasDB() {
		return nil, domain.Validation([]domain.FieldError{
			{Field: "world", Message: fmt.Sprintf("world %q has no save file to back up", world)},
		})
	}

	now := time.Now().UTC()
	if err := os.MkdirAll(paths.Backups, 0o750); err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "create backups directory", err)
	}
	filename := backupFilename(instanceID, world, now, kind)
	fullPath := filepath.Join(paths.Backups, filename)

	manifest := domain.BackupManifest{
		InstanceID:     instanceID,
		World:          world,
		Kind:           kind,
		CreatedAt:      now,
		ValheimBuildID: inst.Status.InstalledBuildID,
		AppVersion:     s.appVersion,
	}
	size, err := writeBackupZip(fullPath, worldsDir, paths.Save, save, manifest)
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "write backup zip", err)
	}

	row := backupRow{InstanceID: instanceID, World: world, Kind: kind, Filename: filename, SizeBytes: size, Note: note, CreatedAt: now}
	id, err := s.insertBackupRow(ctx, row)
	if err != nil {
		_ = os.Remove(fullPath)
		return nil, domain.Wrap(domain.CodeInternal, "save backup record", err)
	}
	row.ID = id

	// Retention runs after every non-manual backup (ARCHITECTURE.md §10);
	// manual and uploaded backups are never auto-deleted, and Upload never
	// calls Create, so this excludes both by construction.
	if kind != domain.BackupManual {
		if err := s.applyRetention(ctx, instanceID, inst.Config); err != nil {
			s.log.Warn("backup retention failed", "instance", instanceID, "err", err)
		}
	}

	// Off-site copy (F-1.4): only when the instance has a target configured
	// and this backup's kind is one it should be copied for. The target
	// itself is resolved later, inside the job, from the same TargetID.
	if rb := inst.Config.RemoteBackup; rb != nil && containsBackupKind(rb.Kinds, kind) {
		if err := s.SetRemoteStatus(ctx, id, "pending", ""); err != nil {
			s.log.Warn("backup: set remote status pending", "instance", instanceID, "id", id, "err", err)
		} else {
			row.RemoteStatus = "pending"
		}
		if _, err := s.enqueueRemoteUpload(ctx, instanceID, id, filename, rb.TargetID, "system"); err != nil {
			s.log.Warn("backup: enqueue off-site upload job", "instance", instanceID, "id", id, "err", err)
		}
	}

	b := row.toDomain()
	return &b, nil
}

// containsBackupKind reports whether k appears in kinds.
func containsBackupKind(kinds []domain.BackupKind, k domain.BackupKind) bool {
	for _, x := range kinds {
		if x == k {
			return true
		}
	}
	return false
}

// EnqueueBackup runs Create as a job so callers get progress/log output and
// the usual job bookkeeping.
func (s *Service) EnqueueBackup(ctx context.Context, instanceID string, kind domain.BackupKind, note, requestedBy string) (*domain.Job, error) {
	if _, err := s.inst.Get(ctx, instanceID); err != nil {
		return nil, err
	}
	if kind == "" {
		kind = domain.BackupManual
	}
	return s.runner.Enqueue(ctx, jobs.Spec{
		Type:        domain.JobBackup,
		InstanceID:  instanceID,
		Title:       "Backup",
		RequestedBy: requestedBy,
	}, func(ctx context.Context, log *jobs.Logger) error {
		b, err := s.Create(ctx, instanceID, kind, note)
		if err != nil {
			return err
		}
		log.Printf("created backup %s (%d bytes)", b.Filename, b.SizeBytes)
		log.SetSummary("filename", b.Filename)
		log.SetSummary("size_bytes", b.SizeBytes)
		if err := s.inst.PublishStatus(ctx, instanceID); err != nil {
			s.log.Warn("backup: publish status after backup", "instance", instanceID, "err", err)
		}
		return nil
	})
}

// PreUpdateBackup runs a pre_update backup and logs it. Its signature
// matches instance.PreUpdateBackupFunc so it plugs directly into WP-05's
// update job.
func (s *Service) PreUpdateBackup(ctx context.Context, instanceID string, log *jobs.Logger) error {
	b, err := s.Create(ctx, instanceID, domain.BackupPreUpdate, "before update")
	if err != nil {
		return fmt.Errorf("pre-update backup: %w", err)
	}
	log.Printf("created pre-update backup %s", b.Filename)
	log.SetSummary("pre_update_backup", b.Filename)
	return nil
}

// ---------------------------------------------------------------- remote upload (F-1.4)

// enqueueRemoteUpload runs the off-site copy of an already-created backup as
// a backup_upload job: mirrors EnqueueBackup's shape. targetID is resolved
// against SetTargets inside the job (not here), so a target added or removed
// between enqueue and run is picked up at run time.
func (s *Service) enqueueRemoteUpload(ctx context.Context, instanceID string, backupID int64, filename, targetID, requestedBy string) (*domain.Job, error) {
	return s.runner.Enqueue(ctx, jobs.Spec{
		Type:        domain.JobBackupUpload,
		InstanceID:  instanceID,
		Title:       fmt.Sprintf("Copy backup %s off-site", filename),
		RequestedBy: requestedBy,
	}, func(ctx context.Context, log *jobs.Logger) error {
		return s.runRemoteUpload(ctx, instanceID, backupID, targetID, log)
	})
}

// runRemoteUpload is the backup_upload job body: resolve the target and the
// backup's zip path, call the uploader, then record the outcome on the
// backup row. The returned error (if any) is also what marks the job failed.
func (s *Service) runRemoteUpload(ctx context.Context, instanceID string, backupID int64, targetID string, log *jobs.Logger) error {
	target, err := s.resolveTarget(ctx, targetID)
	if err != nil {
		_ = s.SetRemoteStatus(ctx, backupID, "failed", err.Error())
		return err
	}
	row, err := s.getBackupRow(ctx, instanceID, backupID)
	if err != nil {
		_ = s.SetRemoteStatus(ctx, backupID, "failed", err.Error())
		return err
	}
	if s.uploader == nil {
		err := fmt.Errorf("off-site uploader is not configured")
		_ = s.SetRemoteStatus(ctx, backupID, "failed", err.Error())
		return err
	}

	// Resolved the same way Open resolves a backup's zip path.
	zipPath := filepath.Join(s.inst.Paths(instanceID).Backups, row.Filename)
	if err := s.uploader.Upload(ctx, target, zipPath, instanceID); err != nil {
		_ = s.SetRemoteStatus(ctx, backupID, "failed", err.Error())
		return err
	}
	log.Printf("copied %s to target %q", row.Filename, target.Name)
	return s.SetRemoteStatus(ctx, backupID, "ok", "")
}

// resolveTarget looks targetID up among the configured targets (SetTargets).
func (s *Service) resolveTarget(ctx context.Context, targetID string) (domain.BackupTarget, error) {
	if s.targetsFn == nil {
		return domain.BackupTarget{}, fmt.Errorf("backup targets are not configured")
	}
	targets, err := s.targetsFn(ctx)
	if err != nil {
		return domain.BackupTarget{}, fmt.Errorf("load backup targets: %w", err)
	}
	for _, t := range targets {
		if t.ID == targetID {
			return t, nil
		}
	}
	return domain.BackupTarget{}, fmt.Errorf("backup target %q not found", targetID)
}

// EnqueueRemoteUpload retries the off-site copy of an existing backup,
// e.g. from the Backups tab's retry button. It verifies backupID belongs to
// instanceID and that the instance still has an off-site target configured,
// then enqueues the same backup_upload job Create would have.
func (s *Service) EnqueueRemoteUpload(ctx context.Context, instanceID string, backupID int64, requestedBy string) (*domain.Job, error) {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	row, err := s.getBackupRow(ctx, instanceID, backupID)
	if err != nil {
		return nil, err
	}
	if inst.Config.RemoteBackup == nil || inst.Config.RemoteBackup.TargetID == "" {
		return nil, domain.Validation([]domain.FieldError{
			{Field: "remote_backup", Message: "instance has no off-site backup target configured"},
		})
	}

	if err := s.SetRemoteStatus(ctx, backupID, "pending", ""); err != nil {
		return nil, err
	}
	return s.enqueueRemoteUpload(ctx, instanceID, backupID, row.Filename, inst.Config.RemoteBackup.TargetID, requestedBy)
}

// Targets returns the configured off-site backup targets with admin-only
// fields (Path/Remote) blanked, for GET /backups/targets: operators pick a
// target by id/name without seeing local paths or rclone remotes.
func (s *Service) Targets(ctx context.Context) ([]domain.BackupTarget, error) {
	if s.targetsFn == nil {
		return []domain.BackupTarget{}, nil
	}
	targets, err := s.targetsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("load backup targets: %w", err)
	}
	out := make([]domain.BackupTarget, len(targets))
	for i, t := range targets {
		out[i] = domain.BackupTarget{ID: t.ID, Name: t.Name, Type: t.Type}
	}
	return out, nil
}

// ---------------------------------------------------------------- list/reconcile

// List returns instanceID's backups newest-first, reconciled against the
// backups directory: rows whose file has vanished get Missing=true, and zip
// files with no row (hand-copied backups, or leftovers from before this
// service tracked them) are inserted as best-effort rows so they stay
// visible.
func (s *Service) List(ctx context.Context, instanceID string) ([]domain.Backup, error) {
	if _, err := s.inst.Get(ctx, instanceID); err != nil {
		return nil, err
	}
	paths := s.inst.Paths(instanceID)

	rows, err := s.listBackupRows(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	known := make(map[string]bool, len(rows))
	for _, r := range rows {
		known[r.Filename] = true
	}

	entries, err := os.ReadDir(paths.Backups)
	if err != nil && !os.IsNotExist(err) {
		return nil, domain.Wrap(domain.CodeInternal, "read backups directory", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".zip") || known[e.Name()] {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		kind, world := parseBackupFilename(instanceID, e.Name())
		nr := backupRow{InstanceID: instanceID, World: world, Kind: kind, Filename: e.Name(), SizeBytes: fi.Size(), CreatedAt: fi.ModTime().UTC()}
		id, err := s.insertBackupRow(ctx, nr)
		if err != nil {
			s.log.Warn("reconcile: insert stray backup row", "instance", instanceID, "file", e.Name(), "err", err)
			continue
		}
		nr.ID = id
		rows = append(rows, nr)
	}

	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.After(rows[j].CreatedAt)
		}
		return rows[i].ID > rows[j].ID
	})

	out := make([]domain.Backup, 0, len(rows))
	for _, r := range rows {
		b := r.toDomain()
		b.Missing = !fileExists(filepath.Join(paths.Backups, r.Filename))
		out = append(out, b)
	}
	return out, nil
}

// ---------------------------------------------------------------- delete/open/upload

// Delete removes both the backup file (if still present) and its row; the
// row is removed even when the file is already gone.
func (s *Service) Delete(ctx context.Context, instanceID string, backupID int64) error {
	r, err := s.getBackupRow(ctx, instanceID, backupID)
	if err != nil {
		return err
	}
	full := filepath.Join(s.inst.Paths(instanceID).Backups, r.Filename)
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return domain.Wrap(domain.CodeInternal, "delete backup file", err)
	}
	return s.deleteBackupRow(ctx, instanceID, backupID)
}

// Open returns the backup's row plus an open reader on its file for download.
func (s *Service) Open(ctx context.Context, instanceID string, backupID int64) (*domain.Backup, io.ReadCloser, error) {
	r, err := s.getBackupRow(ctx, instanceID, backupID)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(filepath.Join(s.inst.Paths(instanceID).Backups, r.Filename)) //nolint:gosec // filename comes from our own DB row, not directly from the request
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, domain.NotFound("backup file")
		}
		return nil, nil, domain.Wrap(domain.CodeInternal, "open backup file", err)
	}
	b := r.toDomain()
	return &b, f, nil
}

// Upload stores a previously-downloaded backup zip: it must have a .zip
// extension and contain either a manifest.json or a worlds_local/*.db entry.
func (s *Service) Upload(ctx context.Context, instanceID, filename string, r io.Reader) (*domain.Backup, error) {
	if _, err := s.inst.Get(ctx, instanceID); err != nil {
		return nil, err
	}

	base := filepath.Base(filename)
	if base == "" || base != filename || !strings.EqualFold(filepath.Ext(base), ".zip") {
		return nil, domain.Validation([]domain.FieldError{{Field: "file", Message: "must be a .zip file"}})
	}

	paths := s.inst.Paths(instanceID)
	if err := os.MkdirAll(paths.Backups, 0o750); err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "create backups directory", err)
	}
	tmp, err := os.CreateTemp(paths.Backups, "upload-*.zip")
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "stage upload", err)
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, r); err != nil { //nolint:gosec // uploaded backup zip, no smaller bound is meaningful here
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return nil, domain.Wrap(domain.CodeInternal, "stage upload", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return nil, domain.Wrap(domain.CodeInternal, "stage upload", err)
	}

	world, err := validateBackupZipContent(tmpName)
	if err != nil {
		_ = os.Remove(tmpName)
		return nil, err
	}

	now := time.Now().UTC()
	finalName := backupFilename(instanceID, world, now, domain.BackupUploaded)
	final := filepath.Join(paths.Backups, finalName)
	for i := 1; fileExists(final); i++ {
		finalName = fmt.Sprintf("%s-%d.zip", strings.TrimSuffix(backupFilename(instanceID, world, now, domain.BackupUploaded), ".zip"), i)
		final = filepath.Join(paths.Backups, finalName)
	}
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return nil, domain.Wrap(domain.CodeInternal, "store uploaded backup", err)
	}
	fi, err := os.Stat(final)
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "stat uploaded backup", err)
	}

	row := backupRow{InstanceID: instanceID, World: world, Kind: domain.BackupUploaded, Filename: finalName, SizeBytes: fi.Size(), CreatedAt: now}
	id, err := s.insertBackupRow(ctx, row)
	if err != nil {
		_ = os.Remove(final)
		return nil, domain.Wrap(domain.CodeInternal, "save backup record", err)
	}
	row.ID = id
	b := row.toDomain()
	return &b, nil
}

// ---------------------------------------------------------------- retention

// applyRetention deletes auto-deletable backups (scheduled, pre_update,
// pre_restore -- never manual or uploaded, see domain.BackupKind.AutoDeletable)
// that fall outside either configured limit: a backup survives only while it
// is both among the newest BackupKeepLast (when that is > 0) and no older
// than BackupKeepDays (when that is > 0); 0 disables that particular limit.
func (s *Service) applyRetention(ctx context.Context, instanceID string, cfg domain.InstanceConfig) error {
	rows, err := s.listBackupRows(ctx, instanceID) // newest first
	if err != nil {
		return err
	}

	var autos []backupRow
	for _, r := range rows {
		if r.Kind.AutoDeletable() {
			autos = append(autos, r)
		}
	}

	now := time.Now().UTC()
	paths := s.inst.Paths(instanceID)
	for i, r := range autos {
		keep := true
		if cfg.BackupKeepLast > 0 && i >= cfg.BackupKeepLast {
			keep = false
		}
		if cfg.BackupKeepDays > 0 && now.Sub(r.CreatedAt) > time.Duration(cfg.BackupKeepDays)*24*time.Hour {
			keep = false
		}
		if keep {
			continue
		}

		full := filepath.Join(paths.Backups, r.Filename)
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			s.log.Warn("retention: remove backup file", "instance", instanceID, "file", r.Filename, "err", err)
			continue
		}
		if err := s.deleteBackupRow(ctx, instanceID, r.ID); err != nil {
			s.log.Warn("retention: delete backup row", "instance", instanceID, "id", r.ID, "err", err)
		}
	}
	return nil
}
