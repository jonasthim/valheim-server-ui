package backup

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
// the world's save files plus manifest.json. Copy order follows
// ARCHITECTURE.md §10: .fwl then .db (mirroring Valheim's own -backups
// behaviour), then their .old siblings if present, then the player lists.
// Returns the final file size.
func writeBackupZip(fullPath, worldsDir, saveDir, world string, manifest domain.BackupManifest) (int64, error) {
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

	var addErr error
	steps := []struct{ disk, arc string }{
		{filepath.Join(worldsDir, world+".fwl"), "worlds_local/" + world + ".fwl"},
		{filepath.Join(worldsDir, world+".db"), "worlds_local/" + world + ".db"},
		{filepath.Join(worldsDir, world+".fwl.old"), "worlds_local/" + world + ".fwl.old"},
		{filepath.Join(worldsDir, world+".db.old"), "worlds_local/" + world + ".db.old"},
		{filepath.Join(saveDir, domain.ListAdmin.FileName()), domain.ListAdmin.FileName()},
		{filepath.Join(saveDir, domain.ListBanned.FileName()), domain.ListBanned.FileName()},
		{filepath.Join(saveDir, domain.ListPermitted.FileName()), domain.ListPermitted.FileName()},
	}
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

// extractBackupZip extracts zipPath's world save files and player lists into
// saveDir, guarding against zip-slip by only ever writing basenames the
// service itself computes (never a path taken from the archive) and by
// double-checking the result stays under saveDir/worlds_local. Anything in
// the archive that is not one of those allowed names -- including
// manifest.json, which is read for its World field but never written to
// disk -- is skipped rather than extracted. Returns the restored world's
// name (from the manifest, or inferred from the .db entry if the manifest is
// absent).
func extractBackupZip(zipPath, saveDir string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open backup zip: %w", err)
	}
	defer func() { _ = zr.Close() }()
	if err := checkArchiveBudget(zr.File, maxArchiveUncompressedBytes); err != nil {
		return "", err
	}

	worldsDir := filepath.Join(saveDir, "worlds_local")
	if err := os.MkdirAll(worldsDir, 0o750); err != nil {
		return "", fmt.Errorf("create worlds directory: %w", err)
	}

	var manifestWorld, dbStem string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := f.Name

		switch {
		case name == "manifest.json":
			manifestWorld = readManifestWorld(f)

		case strings.HasPrefix(name, "worlds_local/"):
			base := filepath.Base(name)
			if base == "" || base == "." || strings.ContainsAny(base, "/\\") || !allowedWorldFileSuffix(base) {
				continue
			}
			dest := filepath.Join(worldsDir, base)
			if !withinDir(worldsDir, dest) {
				continue
			}
			if err := extractZipFile(f, dest); err != nil {
				return "", err
			}
			if strings.HasSuffix(base, ".db") && !strings.HasSuffix(base, ".db.old") {
				dbStem = strings.TrimSuffix(base, ".db")
			}

		case name == domain.ListAdmin.FileName(), name == domain.ListBanned.FileName(), name == domain.ListPermitted.FileName():
			dest := filepath.Join(saveDir, name) //nolint:gosec // G305: name just matched one of three known constant list-file names above, and withinDir double-checks the result below
			if !withinDir(saveDir, dest) {
				continue
			}
			if err := extractZipFile(f, dest); err != nil {
				return "", err
			}

		default:
			// Not a shape a backup may contain; skip it defensively.
		}
	}

	if manifestWorld != "" {
		return manifestWorld, nil
	}
	if dbStem != "" {
		return dbStem, nil
	}
	return "", domain.E(domain.CodeValidationFailed, "backup zip does not contain a recognisable world")
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

	var manifestWorld, dbStem string
	for _, f := range zr.File {
		if f.Name == "manifest.json" {
			manifestWorld = readManifestWorld(f)
			continue
		}
		base := filepath.Base(f.Name)
		lower := strings.ToLower(base)
		if strings.HasPrefix(f.Name, "worlds_local/") && strings.HasSuffix(lower, ".db") && !strings.HasSuffix(lower, ".db.old") {
			dbStem = strings.TrimSuffix(base, filepath.Ext(base))
		}
	}
	if manifestWorld != "" {
		return manifestWorld, nil
	}
	if dbStem != "" {
		return dbStem, nil
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

// extractWorldZipToStaging extracts the single .db/.fwl pair from a world
// import zip into stagingDir under fresh temp names (never the archive's own
// path), returning the world name and the two staged file paths. On any
// error every temp file it created is removed before returning, so a caller
// never has to clean up a partial result.
func extractWorldZipToStaging(zipPath, stagingDir string) (world, dbPath, fwlPath string, err error) {
	zr, zerr := zip.OpenReader(zipPath)
	if zerr != nil {
		return "", "", "", domain.E(domain.CodeValidationFailed, "not a valid zip file")
	}
	defer func() { _ = zr.Close() }()

	var created []string
	cleanup := func() {
		for _, p := range created {
			_ = os.Remove(p)
		}
	}

	type pair struct{ db, fwl string }
	stems := map[string]*pair{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".db" && ext != ".fwl" {
			continue
		}
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		out, oerr := os.CreateTemp(stagingDir, "zippart-*"+ext)
		if oerr != nil {
			cleanup()
			return "", "", "", fmt.Errorf("stage zip entry: %w", oerr)
		}
		created = append(created, out.Name())
		cerr := copyZipEntry(f, out)
		_ = out.Close()
		if cerr != nil {
			cleanup()
			return "", "", "", cerr
		}
		p := stems[stem]
		if p == nil {
			p = &pair{}
			stems[stem] = p
		}
		if ext == ".db" {
			p.db = out.Name()
		} else {
			p.fwl = out.Name()
		}
	}
	if len(stems) != 1 {
		cleanup()
		return "", "", "", domain.E(domain.CodeValidationFailed, "zip must contain exactly one world's .db and .fwl files")
	}
	for stem, p := range stems {
		if p.db == "" || p.fwl == "" {
			cleanup()
			return "", "", "", domain.E(domain.CodeValidationFailed, "zip is missing the .db or .fwl file")
		}
		return stem, p.db, p.fwl, nil
	}
	cleanup()
	return "", "", "", domain.E(domain.CodeValidationFailed, "empty zip")
}

// writeWorldZip streams world's save files (whichever of .db/.fwl exist)
// into w as worlds_local/<world>.{db,fwl}.
func writeWorldZip(w io.Writer, world, dbPath, fwlPath string, hasDB, hasFWL bool) error {
	zw := zip.NewWriter(w)
	if hasFWL {
		if err := addFileToZip(zw, fwlPath, "worlds_local/"+world+".fwl"); err != nil {
			_ = zw.Close()
			return fmt.Errorf("add %s: %w", fwlPath, err)
		}
	}
	if hasDB {
		if err := addFileToZip(zw, dbPath, "worlds_local/"+world+".db"); err != nil {
			_ = zw.Close()
			return fmt.Errorf("add %s: %w", dbPath, err)
		}
	}
	return zw.Close()
}
