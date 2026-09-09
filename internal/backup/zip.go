package backup

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// addFileToZip streams path's contents into zw under arcName.
func addFileToZip(zw *zip.Writer, path, arcName string) error {
	f, err := os.Open(path) //nolint:gosec // path is built from validated instance paths + a controlled world/list-file name
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	hdr, err := zip.FileInfoHeader(fi)
	if err != nil {
		return fmt.Errorf("zip header for %s: %w", path, err)
	}
	hdr.Name = arcName
	hdr.Method = zip.Deflate
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", arcName, err)
	}
	if _, err := io.Copy(w, f); err != nil { //nolint:gosec // backup zip entries, size bounded by save-game files
		return fmt.Errorf("write zip entry %s: %w", arcName, err)
	}
	return nil
}

// writeBackupZip creates fullPath atomically (via a .tmp sibling) containing
// the world's save files plus manifest.json. For the directory layout every
// file under worlds_local/<World>/ is copied (chunk files carry their own
// generation, so the directory is only whole as a set); for the legacy
// layout the copy order follows ARCHITECTURE.md §10: .fwl then .db
// (mirroring Valheim's own -backups behaviour), then their .old siblings if
// present. Player lists come last. Returns the final file size.
func writeBackupZip(fullPath, worldsDir, saveDir string, save worldSave, manifest domain.BackupManifest) (int64, error) {
	tmp := fullPath + ".tmp"
	f, err := os.Create(tmp) //nolint:gosec // fullPath is derived from InstancePaths + a sanitised filename
	if err != nil {
		return 0, fmt.Errorf("create backup zip: %w", err)
	}

	zw := zip.NewWriter(f)
	var files []string
	add := func(diskPath, arcName string) error {
		if !fileExists(diskPath) {
			return nil
		}
		if err := addFileToZip(zw, diskPath, arcName); err != nil {
			return err
		}
		files = append(files, arcName)
		return nil
	}

	steps := worldZipSteps(worldsDir, save)
	steps = append(steps,
		zipStep{filepath.Join(saveDir, domain.ListAdmin.FileName()), domain.ListAdmin.FileName()},
		zipStep{filepath.Join(saveDir, domain.ListBanned.FileName()), domain.ListBanned.FileName()},
		zipStep{filepath.Join(saveDir, domain.ListPermitted.FileName()), domain.ListPermitted.FileName()},
	)
	var addErr error
	for _, step := range steps {
		if addErr = add(step.disk, step.arc); addErr != nil {
			break
		}
	}
	if addErr == nil {
		manifest.Files = files
		var mdata []byte
		if mdata, addErr = json.MarshalIndent(manifest, "", "  "); addErr == nil {
			var mw io.Writer
			if mw, addErr = zw.Create("manifest.json"); addErr == nil {
				_, addErr = mw.Write(mdata)
			}
		}
	}
	if closeErr := zw.Close(); addErr == nil {
		addErr = closeErr
	}
	if fErr := f.Close(); addErr == nil {
		addErr = fErr
	}
	if addErr != nil {
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("write backup zip: %w", addErr)
	}
	if err := os.Rename(tmp, fullPath); err != nil {
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("finalize backup zip: %w", err)
	}
	fi, err := os.Stat(fullPath)
	if err != nil {
		return 0, fmt.Errorf("stat backup zip: %w", err)
	}
	return fi.Size(), nil
}

// zipStep is one file to copy into an archive: its path on disk and its
// entry name.
type zipStep struct{ disk, arc string }

// worldZipSteps lists save's files in copy order with their archive names:
// the legacy pair (+ .old siblings) as worlds_local/<World>.<ext>, then the
// directory layout as worlds_local/<World>/<file>. Files that do not exist
// are skipped by the caller.
func worldZipSteps(worldsDir string, save worldSave) []zipStep {
	world := save.Name
	steps := []zipStep{
		{filepath.Join(worldsDir, world+".fwl"), "worlds_local/" + world + ".fwl"},
		{filepath.Join(worldsDir, world+".db"), "worlds_local/" + world + ".db"},
		{filepath.Join(worldsDir, world+".fwl.old"), "worlds_local/" + world + ".fwl.old"},
		{filepath.Join(worldsDir, world+".db.old"), "worlds_local/" + world + ".db.old"},
	}
	if save.Dir {
		dir := filepath.Join(worldsDir, world)
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !allowedWorldDirFile(e.Name()) {
					continue
				}
				steps = append(steps, zipStep{filepath.Join(dir, e.Name()), "worlds_local/" + world + "/" + e.Name()})
			}
		}
	}
	return steps
}

// allowedWorldDirFile reports whether base is one of the file shapes a
// Valheim 1.0 world directory contains (and therefore a backup may carry).
func allowedWorldDirFile(base string) bool {
	if mainFilePattern.MatchString(base) {
		return true
	}
	return strings.HasSuffix(base, ".chunk") && !strings.ContainsAny(base, "/\\")
}

// allowedWorldFileSuffix reports whether base is one of the world save file
// shapes a backup may contain.
func allowedWorldFileSuffix(base string) bool {
	for _, suf := range []string{".db", ".fwl", ".db.old", ".fwl.old"} {
		if strings.HasSuffix(base, suf) {
			return true
		}
	}
	return false
}

// withinDir reports whether path is (non-strictly) inside dir, guarding
// against zip-slip regardless of how a name was derived.
func withinDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func readManifestWorld(f *zip.File) string {
	rc, err := f.Open()
	if err != nil {
		return ""
	}
	defer func() { _ = rc.Close() }()
	var m domain.BackupManifest
	if err := json.NewDecoder(rc).Decode(&m); err != nil {
		return ""
	}
	return m.World
}

// extractZipFile writes f's content to dest atomically (temp file + rename)
// so an interrupted restore never leaves a half-written save file in place.
func extractZipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %s: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()
	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640) //nolint:gosec // dest is validated against an allow-list of backup content names
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	if err := copyDeclared(out, rc, f.UncompressedSize64); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write %s: %w", dest, err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close %s: %w", dest, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("finalize %s: %w", dest, err)
	}
	return nil
}

// extractBackupZip restores zipPath's world save files and player lists into
// saveDir. The world is identified first (manifest, else inferred from the
// entries), whatever currently exists for that world in either layout is
// removed -- Valheim loads the highest committed generation it finds, so an
// older generation merely placed next to a newer one would be ignored --
// and only then are files written. Zip-slip is prevented by never writing a
// path taken from the archive: destinations are built from the validated
// world name and a basename that matches one of the known save-file shapes,
// then double-checked to stay under worlds_local. Anything else in the
// archive, including manifest.json, is skipped. Returns the restored world's
// name.
func extractBackupZip(zipPath, saveDir string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open backup zip: %w", err)
	}
	defer func() { _ = zr.Close() }()
	if err := checkArchiveBudget(zr.File, maxArchiveUncompressedBytes); err != nil {
		return "", err
	}

	world := zipWorldName(zr.File)
	if world == "" {
		return "", domain.E(domain.CodeValidationFailed, "backup zip does not contain a recognisable world")
	}
	if !validWorldName(world) {
		return "", domain.E(domain.CodeValidationFailed, "backup zip names an invalid world")
	}

	worldsDir := filepath.Join(saveDir, "worlds_local")
	if err := os.MkdirAll(worldsDir, 0o750); err != nil {
		return "", fmt.Errorf("create worlds directory: %w", err)
	}
	if _, err := removeWorldSave(worldsDir, world); err != nil {
		return "", err
	}

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := f.Name
		switch {
		case name == domain.ListAdmin.FileName(), name == domain.ListBanned.FileName(), name == domain.ListPermitted.FileName():
			dest := filepath.Join(saveDir, name) //nolint:gosec // G305: name just matched one of three known constant list-file names above, and withinDir double-checks the result below
			if !withinDir(saveDir, dest) {
				continue
			}
			if err := extractZipFile(f, dest); err != nil {
				return "", err
			}

		case strings.HasPrefix(name, "worlds_local/"+world+"/"):
			base := filepath.Base(name)
			if !allowedWorldDirFile(base) {
				continue
			}
			dir := filepath.Join(worldsDir, world)
			dest := filepath.Join(dir, base)
			if !withinDir(worldsDir, dest) {
				continue
			}
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return "", fmt.Errorf("create world directory: %w", err)
			}
			if err := extractZipFile(f, dest); err != nil {
				return "", err
			}

		case strings.HasPrefix(name, "worlds_local/"):
			base := filepath.Base(name)
			if base == "" || base == "." || strings.ContainsAny(base, "/\\") || !allowedWorldFileSuffix(base) {
				continue
			}
			if !strings.HasPrefix(base, world+".") {
				continue // a stray file for some other world; never restore it
			}
			dest := filepath.Join(worldsDir, base)
			if !withinDir(worldsDir, dest) {
				continue
			}
			if err := extractZipFile(f, dest); err != nil {
				return "", err
			}

		default:
			// manifest.json or something a backup never contains; skip.
		}
	}
	return world, nil
}

// zipWorldName identifies the world an archive is for: manifest.json's
// world field when present, else the stem of a worlds_local/<World>.db
// entry, else the directory of a worlds_local/<World>/_main.<N>.db2 entry.
// Returns "" when none applies.
func zipWorldName(files []*zip.File) string {
	var dbStem, dirWorld string
	for _, f := range files {
		if f.Name == "manifest.json" {
			if w := readManifestWorld(f); w != "" {
				return w
			}
			continue
		}
		if !strings.HasPrefix(f.Name, "worlds_local/") {
			continue
		}
		rest := strings.TrimPrefix(f.Name, "worlds_local/")
		if i := strings.Index(rest, "/"); i >= 0 {
			base := rest[i+1:]
			if m := mainFilePattern.FindStringSubmatch(base); m != nil && m[2] == "db2" && dirWorld == "" {
				dirWorld = rest[:i]
			}
			continue
		}
		lower := strings.ToLower(rest)
		if strings.HasSuffix(lower, ".db") && dbStem == "" {
			dbStem = strings.TrimSuffix(rest, filepath.Ext(rest))
		}
	}
	if dbStem != "" {
		return dbStem
	}
	return dirWorld
}

// validateBackupZipContent reports the world a backup zip is for, or a
// validation error if it looks like neither a backup nor a usable world
// archive (used by Upload).
func validateBackupZipContent(zipPath string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", domain.E(domain.CodeValidationFailed, "not a valid zip file")
	}
	defer func() { _ = zr.Close() }()
	if err := checkArchiveBudget(zr.File, maxArchiveUncompressedBytes); err != nil {
		return "", err
	}
	if world := zipWorldName(zr.File); world != "" {
		return world, nil
	}
	return "", domain.E(domain.CodeValidationFailed, "zip does not contain manifest.json or a worlds_local/*.db file")
}

// copyZipEntry streams f's content into out.
func copyZipEntry(f *zip.File, out *os.File) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %s: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()
	if err := copyDeclared(out, rc, f.UncompressedSize64); err != nil {
		return fmt.Errorf("read zip entry %s: %w", f.Name, err)
	}
	return nil
}

// maxArchiveUncompressedBytes caps the declared uncompressed total of a
// backup or world archive (world databases are hundreds of MB, not tens of GB).
const maxArchiveUncompressedBytes = 16 << 30

// checkArchiveBudget rejects archives whose declared uncompressed total
// exceeds the budget before anything is written.
func checkArchiveBudget(files []*zip.File, budget uint64) error {
	var total uint64
	for _, f := range files {
		total += f.UncompressedSize64
		if total > budget {
			return domain.Ef(domain.CodeValidationFailed, "archive expands to more than %d bytes", budget)
		}
	}
	return nil
}

// copyDeclared copies at most the declared entry size and fails when the
// entry inflates past it (a lying header, i.e. a zip bomb).
func copyDeclared(dst io.Writer, src io.Reader, declared uint64) error {
	limit := int64(declared) //nolint:gosec // declared comes from a zip header; values past int64 are rejected by the budget check
	n, err := io.Copy(dst, io.LimitReader(src, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return errors.New("entry larger than its declared size")
	}
	return nil
}

// extractWorldZipToStaging extracts exactly one world from a world import
// zip into stagingDir under fresh temp names (never the archive's own
// path). The archive may hold a legacy <World>.db + <World>.fwl pair, or a
// Valheim 1.0 world directory (<World>/_main.<N>.* plus chunk files, with
// or without a leading worlds_local/), which must contain at least one
// committed generation. On any error every temp file it created is removed
// before returning, so a caller never has to clean up a partial result.
func extractWorldZipToStaging(zipPath, stagingDir string) (stagedWorld, error) {
	zr, zerr := zip.OpenReader(zipPath)
	if zerr != nil {
		return stagedWorld{}, domain.E(domain.CodeValidationFailed, "not a valid zip file")
	}
	defer func() { _ = zr.Close() }()
	if err := checkArchiveBudget(zr.File, maxArchiveUncompressedBytes); err != nil {
		return stagedWorld{}, err
	}

	var created []string
	cleanup := func() {
		for _, p := range created {
			_ = os.Remove(p)
		}
	}
	fail := func(err error) (stagedWorld, error) {
		cleanup()
		return stagedWorld{}, err
	}
	stage := func(f *zip.File, pattern string) (string, error) {
		out, err := os.CreateTemp(stagingDir, pattern)
		if err != nil {
			return "", fmt.Errorf("stage zip entry: %w", err)
		}
		created = append(created, out.Name())
		cerr := copyZipEntry(f, out)
		_ = out.Close()
		if cerr != nil {
			return "", cerr
		}
		return out.Name(), nil
	}

	type pair struct{ db, fwl string }
	pairs := map[string]*pair{}
	dirs := map[string]map[string]string{}
	committed := map[string]map[int][3]bool{} // world -> gen -> {ok, db2, fwl2}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(f.Name, "worlds_local/")
		base := filepath.Base(name)
		if strings.ContainsAny(base, "/\\") || base == "." || base == ".." {
			continue
		}
		if i := strings.LastIndex(name, "/"); i >= 0 {
			// <World>/<file>: directory layout.
			world := filepath.Base(name[:i])
			if !validWorldName(world) || !allowedWorldDirFile(base) {
				continue
			}
			p, err := stage(f, "zippart-*")
			if err != nil {
				return fail(err)
			}
			if dirs[world] == nil {
				dirs[world] = map[string]string{}
				committed[world] = map[int][3]bool{}
			}
			dirs[world][base] = p
			if m := mainFilePattern.FindStringSubmatch(base); m != nil {
				n, _ := strconv.Atoi(m[1])
				flags := committed[world][n]
				switch m[2] {
				case "ok":
					flags[0] = true
				case "db2":
					flags[1] = true
				case "fwl2":
					flags[2] = true
				}
				committed[world][n] = flags
			}
			continue
		}
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".db" && ext != ".fwl" {
			continue
		}
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		p, err := stage(f, "zippart-*"+ext)
		if err != nil {
			return fail(err)
		}
		pr := pairs[stem]
		if pr == nil {
			pr = &pair{}
			pairs[stem] = pr
		}
		if ext == ".db" {
			pr.db = p
		} else {
			pr.fwl = p
		}
	}

	switch {
	case len(pairs)+len(dirs) == 0:
		return fail(domain.E(domain.CodeValidationFailed, "zip does not contain a world (a .db + .fwl pair, or a world directory with _main.<N>.db2/.fwl2/.ok files)"))
	case len(pairs)+len(dirs) > 1:
		return fail(domain.E(domain.CodeValidationFailed, "zip must contain exactly one world"))
	}
	for stem, pr := range pairs {
		if pr.db == "" || pr.fwl == "" {
			return fail(domain.E(domain.CodeValidationFailed, "zip is missing the .db or .fwl file"))
		}
		return stagedWorld{Name: stem, DB: pr.db, FWL: pr.fwl}, nil
	}
	for world, files := range dirs {
		ok := false
		for _, flags := range committed[world] {
			if flags[0] && flags[1] && flags[2] {
				ok = true
			}
		}
		if !ok {
			return fail(domain.E(domain.CodeValidationFailed, "world directory has no committed save (_main.<N>.db2, .fwl2 and .ok for the same N)"))
		}
		return stagedWorld{Name: world, DirFiles: files}, nil
	}
	return fail(domain.E(domain.CodeValidationFailed, "empty zip"))
}

// writeWorldZip streams save's files (whichever exist, either layout) into
// w under worlds_local/.
func writeWorldZip(w io.Writer, worldsDir string, save worldSave) error {
	zw := zip.NewWriter(w)
	for _, step := range worldZipSteps(worldsDir, save) {
		if !fileExists(step.disk) {
			continue
		}
		if err := addFileToZip(zw, step.disk, step.arc); err != nil {
			_ = zw.Close()
			return fmt.Errorf("add %s: %w", step.disk, err)
		}
	}
	return zw.Close()
}
