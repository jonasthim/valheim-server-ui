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

// validWorldName guards every user-supplied world name against path
// traversal before it is ever joined onto a filesystem path.
func validWorldName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return filepath.Base(name) == name
}

// ---------------------------------------------------------------- list

// ListWorlds scans the instance's worlds_local directory for *.fwl/*.db
// pairs.
func (s *Service) ListWorlds(ctx context.Context, instanceID string) ([]domain.World, error) {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	dir := s.inst.Paths(instanceID).WorldsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.World{}, nil
		}
		return nil, domain.Wrap(domain.CodeInternal, "read worlds directory", err)
	}

	type acc struct {
		hasDB, hasFWL bool
		size          int64
		modified      time.Time
	}
	byName := map[string]*acc{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".db" && ext != ".fwl" {
			continue // .old siblings deliberately excluded from the world list
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if isValheimBackupStem(stem) {
			continue // Valheim's own rolling copies (-backups), not selectable worlds
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		a := byName[stem]
		if a == nil {
			a = &acc{}
			byName[stem] = a
		}
		if ext == ".db" {
			a.hasDB = true
		} else {
			a.hasFWL = true
		}
		a.size += fi.Size()
		if fi.ModTime().After(a.modified) {
			a.modified = fi.ModTime()
		}
	}

	out := make([]domain.World, 0, len(byName))
	for name, a := range byName {
		out = append(out, domain.World{
			Name:       name,
			Active:     name == inst.Config.World,
			SizeBytes:  a.size,
			ModifiedAt: a.modified.UTC(),
			HasDB:      a.hasDB,
			HasFWL:     a.hasFWL,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ---------------------------------------------------------------- import

// importFile is one uploaded part already staged to a temp file, so it
// survives past the HTTP handler returning (its multipart reader would not).
type importFile struct {
	origName string
	tmpPath  string
}

// EnqueueWorldImport validates and stages the uploaded file(s) synchronously
// -- so the caller's io.Readers (multipart form parts that die with the
// request) are fully consumed, and a malformed upload (e.g. a zip missing
// its .fwl) is rejected with a validation error before this returns, rather
// than surfacing only as a failed job -- then runs the actual write as a
// world_import job.
func (s *Service) EnqueueWorldImport(ctx context.Context, instanceID string, files map[string]io.Reader, overwrite bool, requestedBy string) (*domain.Job, error) {
	if _, err := s.inst.Get(ctx, instanceID); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, domain.Validation([]domain.FieldError{{Field: "files", Message: "at least one file is required"}})
	}

	asZip := false
	if len(files) == 1 {
		for name := range files {
			if strings.EqualFold(filepath.Ext(name), ".zip") {
				asZip = true
			}
		}
	}
	if !asZip {
		if err := validateWorldFilePair(files); err != nil {
			return nil, err
		}
	}

	stagingDir := filepath.Join(s.inst.Paths(instanceID).Save, ".staging")
	if err := os.MkdirAll(stagingDir, 0o750); err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "create staging directory", err)
	}

	// Stage every input reader to a temp file first, regardless of shape.
	type stagedInput struct{ origName, tmpPath string }
	inputs := make([]stagedInput, 0, len(files))
	cleanupInputs := func() {
		for _, in := range inputs {
			_ = os.Remove(in.tmpPath)
		}
	}
	for name, r := range files {
		base := filepath.Base(name)
		tmp, err := os.CreateTemp(stagingDir, "upload-*")
		if err != nil {
			cleanupInputs()
			return nil, domain.Wrap(domain.CodeInternal, "stage upload", err)
		}
		if _, err := io.Copy(tmp, r); err != nil { //nolint:gosec // uploaded world save file, no smaller bound is meaningful here
			_ = tmp.Close()
			cleanupInputs()
			return nil, domain.Wrap(domain.CodeInternal, "stage upload", err)
		}
		if err := tmp.Close(); err != nil {
			cleanupInputs()
			return nil, domain.Wrap(domain.CodeInternal, "stage upload", err)
		}
		inputs = append(inputs, stagedInput{origName: base, tmpPath: tmp.Name()})
	}

	// Normalise to a .db+.fwl pair up front: for a zip, extract (and fully
	// validate) it right now so a bad archive is rejected synchronously
	// instead of only failing the job later.
	var staged []importFile
	if asZip {
		world, dbPath, fwlPath, err := extractWorldZipToStaging(inputs[0].tmpPath, stagingDir)
		_ = os.Remove(inputs[0].tmpPath)
		if err != nil {
			return nil, err
		}
		staged = []importFile{
			{origName: world + ".db", tmpPath: dbPath},
			{origName: world + ".fwl", tmpPath: fwlPath},
		}
	} else {
		staged = make([]importFile, 0, len(inputs))
		for _, in := range inputs {
			staged = append(staged, importFile(in))
		}
	}

	cleanup := func() {
		for _, sf := range staged {
			_ = os.Remove(sf.tmpPath)
		}
	}
	job, err := s.runner.Enqueue(ctx, jobs.Spec{
		Type:        domain.JobWorldImport,
		InstanceID:  instanceID,
		Title:       "Import world",
		RequestedBy: requestedBy,
	}, func(ctx context.Context, log *jobs.Logger) error {
		defer cleanup()
		return s.runWorldImport(ctx, instanceID, staged, overwrite, log)
	})
	if err != nil {
		cleanup()
		return nil, err
	}
	return job, nil
}

// validateWorldFilePair checks that files is exactly one matching .db+.fwl
// pair (same stem).
func validateWorldFilePair(files map[string]io.Reader) error {
	if len(files) != 2 {
		return domain.Validation([]domain.FieldError{{Field: "files", Message: "provide a matching .db + .fwl pair, or a single zip file"}})
	}
	stems := map[string]map[string]bool{}
	for name := range files {
		base := filepath.Base(name)
		if base == "" || base != name {
			return domain.Validation([]domain.FieldError{{Field: "files", Message: "invalid file name " + name}})
		}
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".db" && ext != ".fwl" {
			return domain.Validation([]domain.FieldError{{Field: "files", Message: "unexpected file " + base}})
		}
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		if stems[stem] == nil {
			stems[stem] = map[string]bool{}
		}
		stems[stem][ext] = true
	}
	if len(stems) != 1 {
		return domain.Validation([]domain.FieldError{{Field: "files", Message: "the .db and .fwl files must share the same world name"}})
	}
	for _, exts := range stems {
		if !exts[".db"] || !exts[".fwl"] {
			return domain.Validation([]domain.FieldError{{Field: "files", Message: "both a .db and a .fwl file are required"}})
		}
	}
	return nil
}

func worldFilesExist(worldsDir, world string) bool {
	return fileExists(filepath.Join(worldsDir, world+".db")) || fileExists(filepath.Join(worldsDir, world+".fwl"))
}

// runWorldImport writes staged -- always a normalised .db+.fwl pair by the
// time EnqueueWorldImport hands it off, whether the original upload was a
// pair or a zip -- into the instance's worlds_local directory.
func (s *Service) runWorldImport(ctx context.Context, instanceID string, staged []importFile, overwrite bool, log *jobs.Logger) error {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return err
	}
	worldsDir := s.inst.Paths(instanceID).WorldsDir()

	var world, dbSrc, fwlSrc string
	for _, sf := range staged {
		ext := strings.ToLower(filepath.Ext(sf.origName))
		stem := strings.TrimSuffix(sf.origName, filepath.Ext(sf.origName))
		world = stem
		if ext == ".db" {
			dbSrc = sf.tmpPath
		} else {
			fwlSrc = sf.tmpPath
		}
	}
	if world == "" || !validWorldName(world) {
		return domain.Validation([]domain.FieldError{{Field: "files", Message: "could not determine a valid world name"}})
	}

	if worldFilesExist(worldsDir, world) && !overwrite {
		return domain.Ef(domain.CodeConflict, "world %q already exists", world)
	}
	isActive := world == inst.Config.World
	if isActive && isBusyState(inst.Status.State) {
		return domain.Ef(domain.CodeInstanceRunning, "cannot overwrite the active world %q while running", world)
	}

	if err := os.MkdirAll(worldsDir, 0o750); err != nil {
		return domain.Wrap(domain.CodeInternal, "create worlds directory", err)
	}
	if err := atomicMove(dbSrc, filepath.Join(worldsDir, world+".db")); err != nil {
		return err
	}
	if err := atomicMove(fwlSrc, filepath.Join(worldsDir, world+".fwl")); err != nil {
		return err
	}

	log.Printf("imported world %q", world)
	log.SetSummary("world", world)

	if isActive {
		if err := s.inst.MarkPendingRestart(ctx, instanceID); err != nil {
			return err
		}
	}
	if err := s.inst.PublishStatus(ctx, instanceID); err != nil {
		s.log.Warn("backup: publish status after world import", "instance", instanceID, "err", err)
	}
	return nil
}

// atomicMove moves src to dst, falling back to copy+remove if they are on
// different filesystems.
func atomicMove(src, dst string) error {
	if src == "" {
		return domain.E(domain.CodeInternal, "missing staged file")
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src) //nolint:gosec // src is a temp file this service just staged
	if err != nil {
		return domain.Wrap(domain.CodeInternal, "open staged file", err)
	}
	defer func() { _ = in.Close() }()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp) //nolint:gosec // dst is derived from InstancePaths + a validated world name
	if err != nil {
		return domain.Wrap(domain.CodeInternal, "create target file", err)
	}
	if _, err := io.Copy(out, in); err != nil { //nolint:gosec // staged world save file, size bounded by the original upload
		_ = out.Close()
		_ = os.Remove(tmp)
		return domain.Wrap(domain.CodeInternal, "copy staged file", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return domain.Wrap(domain.CodeInternal, "close target file", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return domain.Wrap(domain.CodeInternal, "finalize target file", err)
	}
	_ = os.Remove(src)
	return nil
}

// ---------------------------------------------------------------- delete/export

// DeleteWorld removes an inactive world's .db/.fwl (and .old siblings).
func (s *Service) DeleteWorld(ctx context.Context, instanceID, world string) error {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return err
	}
	if !validWorldName(world) {
		return domain.Validation([]domain.FieldError{{Field: "world", Message: "invalid world name"}})
	}
	if world == inst.Config.World {
		return domain.Ef(domain.CodeConflict, "cannot delete the active world %q", world)
	}

	dir := s.inst.Paths(instanceID).WorldsDir()
	removed := false
	for _, suffix := range []string{".db", ".fwl", ".db.old", ".fwl.old"} {
		p := filepath.Join(dir, world+suffix)
		if err := os.Remove(p); err == nil {
			removed = true
		} else if !os.IsNotExist(err) {
			return domain.Wrap(domain.CodeInternal, "delete world file", err)
		}
	}
	if !removed {
		return domain.NotFound("world")
	}
	return nil
}

// ExportWorld streams a zip of world's .db/.fwl files to w. Existence and
// the world name are validated before anything is written to w, so a caller
// that has already set response headers can still turn an error into a
// proper HTTP status.
func (s *Service) ExportWorld(ctx context.Context, instanceID, world string, w io.Writer) error {
	if _, err := s.inst.Get(ctx, instanceID); err != nil {
		return err
	}
	if !validWorldName(world) {
		return domain.Validation([]domain.FieldError{{Field: "world", Message: "invalid world name"}})
	}
	dir := s.inst.Paths(instanceID).WorldsDir()
	dbPath := filepath.Join(dir, world+".db")
	fwlPath := filepath.Join(dir, world+".fwl")
	hasDB, hasFWL := fileExists(dbPath), fileExists(fwlPath)
	if !hasDB && !hasFWL {
		return domain.NotFound("world")
	}
	if err := writeWorldZip(w, world, dbPath, fwlPath, hasDB, hasFWL); err != nil {
		return fmt.Errorf("export world %q: %w", world, err)
	}
	return nil
}

// isValheimBackupStem reports whether a world file stem is one of Valheim's
// own automatic copies (e.g. "Midgard_backup_auto-20260909144803",
// "_backup_cloud-", "_backup_restore-"), which the manager lists under
// neither Worlds nor Backups: they are not loadable by name and the manager
// keeps its own zip backups.
func isValheimBackupStem(stem string) bool {
	return strings.Contains(stem, "_backup_auto-") ||
		strings.Contains(stem, "_backup_cloud-") ||
		strings.Contains(stem, "_backup_restore-")
}
