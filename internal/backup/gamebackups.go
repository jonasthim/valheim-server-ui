package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

const (
	gameBackupLayoutDirectory = "directory"
	gameBackupLayoutLegacy    = "legacy"
	gameBackupTimeLayout      = "20060102150405"
)

// gameBackupMarkers maps each separator Valheim puts between a world name and
// the timestamp to the kind it denotes.
var gameBackupMarkers = []struct{ marker, kind string }{
	{"_backup_auto-", "auto"},
	{"_backup_cloud-", "cloud"},
	{"_backup_restore-", "restore"},
}

// parseGameBackupStem splits "<World>_backup_<kind>-<ts>" at the LAST marker
// occurrence, so a world whose own name contains "_backup_" still parses.
// ts is the zero time when the suffix is not a valid timestamp.
func parseGameBackupStem(stem string) (world, kind string, ts time.Time, ok bool) {
	best, bestKind, bestLen := -1, "", 0
	for _, m := range gameBackupMarkers {
		if i := strings.LastIndex(stem, m.marker); i > best {
			best, bestKind, bestLen = i, m.kind, len(m.marker)
		}
	}
	if best < 0 {
		return "", "", time.Time{}, false
	}
	world = stem[:best]
	if !validWorldName(world) {
		return "", "", time.Time{}, false
	}
	if t, err := time.ParseInLocation(gameBackupTimeLayout, stem[best+bestLen:], time.UTC); err == nil {
		ts = t
	}
	return world, bestKind, ts, true
}

// scanGameBackups lists Valheim's rolling copies in the worlds directory,
// newest first. A missing directory yields an empty slice. Symlinks are
// never followed.
func scanGameBackups(dir string) ([]domain.GameBackup, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.GameBackup{}, nil
		}
		return nil, fmt.Errorf("read worlds directory: %w", err)
	}

	out := []domain.GameBackup{}
	type legacyCopy struct {
		size  int64
		mtime time.Time
		db    bool
	}
	legacy := map[string]*legacyCopy{}

	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := e.Name()
		if e.IsDir() {
			world, kind, ts, ok := parseGameBackupStem(name)
			if !ok {
				continue
			}
			w, err := scanWorldDir(filepath.Join(dir, name), name)
			if err != nil {
				return nil, err
			}
			gb := domain.GameBackup{Name: name, World: world, Kind: kind, Layout: gameBackupLayoutDirectory}
			mtime := time.Time{}
			if w != nil {
				gb.SizeBytes = w.SizeBytes
				mtime = w.ModifiedAt
				gb.Restorable = w.Generation > 0 && w.DirDB
			} else if fi, err := e.Info(); err == nil {
				mtime = fi.ModTime()
			}
			gb.CreatedAt = createdAt(ts, mtime)
			out = append(out, gb)
			continue
		}

		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".db" && ext != ".fwl" {
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if _, _, _, ok := parseGameBackupStem(stem); !ok {
			continue
		}
		fi, err := e.Info()
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		lc := legacy[stem]
		if lc == nil {
			lc = &legacyCopy{}
			legacy[stem] = lc
		}
		lc.size += fi.Size()
		if fi.ModTime().After(lc.mtime) {
			lc.mtime = fi.ModTime()
		}
		if ext == ".db" {
			lc.db = true
		}
	}

	for stem, lc := range legacy {
		world, kind, ts, _ := parseGameBackupStem(stem)
		out = append(out, domain.GameBackup{
			Name: stem, World: world, Kind: kind, Layout: gameBackupLayoutLegacy,
			CreatedAt: createdAt(ts, lc.mtime), SizeBytes: lc.size, Restorable: lc.db,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func createdAt(ts, mtime time.Time) time.Time {
	if ts.IsZero() {
		return mtime.UTC()
	}
	return ts.UTC()
}

// ListGameBackups lists Valheim's own rolling world copies for an instance.
func (s *Service) ListGameBackups(ctx context.Context, instanceID string) ([]domain.GameBackup, error) {
	if _, err := s.inst.Get(ctx, instanceID); err != nil {
		return nil, err
	}
	out, err := scanGameBackups(s.inst.Paths(instanceID).WorldsDir())
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "scan game backups", err)
	}
	return out, nil
}

// EnqueueGameBackupRestore restores one of the game's rolling copies over its
// world as a restore job. The copy itself is left in place.
func (s *Service) EnqueueGameBackupRestore(ctx context.Context, instanceID, name string, stopIfRunning bool, requestedBy string) (*domain.Job, error) {
	if _, err := s.inst.Get(ctx, instanceID); err != nil {
		return nil, err
	}
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || !isValheimBackupStem(name) {
		return nil, domain.NotFound("game backup")
	}
	copyInfo, err := s.findGameBackup(instanceID, name)
	if err != nil {
		return nil, err
	}
	if !copyInfo.Restorable {
		return nil, domain.E(domain.CodeValidationFailed, "this copy has no committed save")
	}
	return s.runner.Enqueue(ctx, jobs.Spec{
		Type:        domain.JobRestore,
		InstanceID:  instanceID,
		Title:       "Restore game backup " + name,
		RequestedBy: requestedBy,
	}, func(ctx context.Context, log *jobs.Logger) error {
		return s.runGameBackupRestore(ctx, instanceID, name, stopIfRunning, log)
	})
}

func (s *Service) findGameBackup(instanceID, name string) (domain.GameBackup, error) {
	copies, err := scanGameBackups(s.inst.Paths(instanceID).WorldsDir())
	if err != nil {
		return domain.GameBackup{}, domain.Wrap(domain.CodeInternal, "scan game backups", err)
	}
	for _, c := range copies {
		if c.Name == name {
			return c, nil
		}
	}
	return domain.GameBackup{}, domain.NotFound("game backup")
}

func (s *Service) runGameBackupRestore(ctx context.Context, instanceID, name string, stopIfRunning bool, log *jobs.Logger) error {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return err
	}
	gb, err := s.findGameBackup(instanceID, name)
	if err != nil {
		return err
	}
	if !gb.Restorable {
		return domain.E(domain.CodeValidationFailed, "this copy has no committed save")
	}

	wasRunning := isBusyState(inst.Status.State)
	if wasRunning && !stopIfRunning {
		return domain.Ef(domain.CodeInstanceRunning, "instance %q is running; retry with stop_if_running", instanceID)
	}
	if wasRunning {
		log.Printf("stopping instance before restore")
		if _, err := s.inst.Stop(ctx, instanceID); err != nil {
			return fmt.Errorf("stop instance: %w", err)
		}
		if err := s.waitStopped(ctx, instanceID); err != nil {
			return err
		}
	}

	paths := s.inst.Paths(instanceID)
	worldsDir := paths.WorldsDir()

	// Protect the world that is about to be replaced, which is the copy's
	// own world and need not be the active one (the active world is not
	// touched when they differ). A world with no save yet has nothing to
	// protect, and Create would refuse it.
	target, err := scanWorld(worldsDir, gb.World)
	if err != nil {
		return fmt.Errorf("scan world %q: %w", gb.World, err)
	}
	if target.HasDB() {
		log.Printf("creating pre-restore backup of world %q", gb.World)
		pre, err := s.createForWorld(ctx, instanceID, gb.World, domain.BackupPreRestore, "before restoring game backup "+name)
		if err != nil {
			return fmt.Errorf("pre-restore backup: %w", err)
		}
		log.SetSummary("pre_restore_backup", pre.Filename)
	} else {
		log.Printf("world %q has no save data yet; skipping the pre-restore backup", gb.World)
	}

	if err := stageAndSwapGameBackup(worldsDir, paths.Save, gb); err != nil {
		return fmt.Errorf("restore game backup %s: %w", name, err)
	}
	log.Printf("restored world %q from game backup %s", gb.World, name)
	log.SetSummary("restored_world", gb.World)

	if gb.World != inst.Config.World {
		newCfg := inst.Config
		newCfg.World = gb.World
		if _, err := s.inst.Update(ctx, instanceID, nil, &newCfg, nil); err != nil {
			return fmt.Errorf("switch active world to %q: %w", gb.World, err)
		}
		log.Printf("active world switched to %q", gb.World)
	}

	if wasRunning {
		log.Printf("starting instance after restore")
		if _, err := s.inst.Start(ctx, instanceID); err != nil {
			return fmt.Errorf("start instance: %w", err)
		}
	}
	if err := s.inst.PublishStatus(ctx, instanceID); err != nil {
		s.log.Warn("backup: publish status after game backup restore", "instance", instanceID, "err", err)
	}
	return nil
}

// stageAndSwapGameBackup copies the rolling copy into a staging path on the
// same filesystem, then replaces the world's save with it. The copy is never
// modified.
func stageAndSwapGameBackup(worldsDir, saveDir string, gb domain.GameBackup) (err error) {
	src := filepath.Join(worldsDir, gb.Name)
	if gb.Layout == gameBackupLayoutLegacy {
		for _, ext := range []string{".db", ".fwl"} {
			if fi, statErr := os.Lstat(src + ext); statErr != nil || !fi.Mode().IsRegular() {
				return domain.E(domain.CodeValidationFailed, "this copy is incomplete: it needs both a .db and a .fwl file")
			}
		}
	}

	if err := os.MkdirAll(saveDir, 0o750); err != nil {
		return fmt.Errorf("create save directory: %w", err)
	}
	staging := filepath.Join(saveDir, fmt.Sprintf(".restore-%s-%d", gb.World, time.Now().UnixNano()))
	defer func() {
		if err != nil {
			_ = os.RemoveAll(staging)
		}
	}()

	if gb.Layout == gameBackupLayoutLegacy {
		if err := os.MkdirAll(staging, 0o750); err != nil {
			return fmt.Errorf("create staging directory: %w", err)
		}
		for _, ext := range []string{".db", ".fwl"} {
			if err := copyRegularFile(src+ext, filepath.Join(staging, gb.World+ext)); err != nil {
				return err
			}
		}
	} else if err := copyTreeRegular(src, staging); err != nil {
		return err
	}

	if err := os.MkdirAll(worldsDir, 0o750); err != nil {
		return fmt.Errorf("create worlds directory: %w", err)
	}
	if _, err := removeWorldSave(worldsDir, gb.World); err != nil {
		return err
	}
	if gb.Layout == gameBackupLayoutLegacy {
		for _, ext := range []string{".db", ".fwl"} {
			if err := os.Rename(filepath.Join(staging, gb.World+ext), filepath.Join(worldsDir, gb.World+ext)); err != nil {
				return fmt.Errorf("move %s into place: %w", ext, err)
			}
		}
		_ = os.RemoveAll(staging)
		return nil
	}
	if err := os.Rename(staging, filepath.Join(worldsDir, gb.World)); err != nil {
		return fmt.Errorf("move world directory into place: %w", err)
	}
	return nil
}

// copyTreeRegular recursively copies regular files and directories from src
// to dst, skipping symlinks and anything else.
func copyTreeRegular(src, dst string) error {
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	for _, e := range entries {
		s, d := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		switch {
		case e.IsDir():
			if err := copyTreeRegular(s, d); err != nil {
				return err
			}
		case e.Type().IsRegular():
			if err := copyRegularFile(s, d); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyRegularFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // src is inside the worlds directory, name validated
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Base(src), err)
	}
	defer func() { _ = in.Close() }()
	fi, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", filepath.Base(src), err)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fi.Mode().Perm()|0o600) //nolint:gosec // dst is inside our staging directory
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(dst), err)
	}
	if _, err := io.Copy(out, in); err != nil { //nolint:gosec // world save file, size bounded by the source
		_ = out.Close()
		return fmt.Errorf("copy %s: %w", filepath.Base(src), err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", filepath.Base(dst), err)
	}
	return nil
}
