package mods

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// slugPattern validates a single path segment supplied by a caller (owner,
// package name, uploaded file name): no path separators, no "..", not empty.
var slugPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validateSlug(s string) error {
	if s == "" || s == "." || s == ".." || !slugPattern.MatchString(s) {
		return domain.Ef(domain.CodeValidationFailed, "invalid name %q", s)
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// writeFileAtomic writes data to path via a temp file in the same directory
// followed by a rename, so readers never observe a partial file.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", closeErr)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename into place: %w", err)
	}
	return nil
}

// verifyZip opens p and confirms it is a well-formed zip archive.
func verifyZip(p string) error {
	r, err := zip.OpenReader(p)
	if err != nil {
		return err
	}
	return r.Close()
}

// safeZipEntryPath validates and cleans a zip entry name, rejecting zip-slip
// attempts (absolute paths, ".." segments). The returned path always uses "/"
// separators.
func safeZipEntryPath(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(name, "/") || (len(name) >= 2 && name[1] == ':') {
		return "", fmt.Errorf("zip-slip: absolute entry path %q", name)
	}
	clean := path.Clean(name)
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("zip-slip: entry escapes archive root: %q", name)
	}
	return clean, nil
}

// commonTopFolder returns the single top-level path segment shared by every
// entry in files, or "" if there is no such single wrapper folder (i.e. at
// least one entry sits at the archive root).
func commonTopFolder(names []string) string {
	common := ""
	first := true
	for _, name := range names {
		if name == "" {
			continue // pure directory marker for the wrapper itself
		}
		idx := strings.IndexByte(name, '/')
		if idx < 0 {
			return "" // a file lives at the archive root: no single wrapper
		}
		seg := name[:idx]
		if first {
			common = seg
			first = false
		} else if seg != common {
			return ""
		}
	}
	return common
}

// destinationFor maps one package zip entry (relPath, already stripped of any
// wrapper folder) to its destination path relative to the instance's server
// directory, per ARCHITECTURE.md §12's extraction rules. owner/name identify
// the managed mod for the plugins/patchers/core/"loose file" cases.
func destinationFor(relPath, owner, name string) string {
	modDir := owner + "-" + name
	switch {
	case relPath == "BepInEx" || strings.HasPrefix(relPath, "BepInEx/"):
		return relPath
	case relPath == "plugins" || strings.HasPrefix(relPath, "plugins/"):
		return path.Join("BepInEx", "plugins", modDir, strings.TrimPrefix(relPath, "plugins/"))
	case relPath == "patchers" || strings.HasPrefix(relPath, "patchers/"):
		return path.Join("BepInEx", "patchers", modDir, strings.TrimPrefix(relPath, "patchers/"))
	case relPath == "core" || strings.HasPrefix(relPath, "core/"):
		return path.Join("BepInEx", "core", modDir, strings.TrimPrefix(relPath, "core/"))
	case relPath == "config" || strings.HasPrefix(relPath, "config/"):
		return path.Join("BepInEx", "config", strings.TrimPrefix(relPath, "config/"))
	case relPath == "manifest.json" || relPath == "icon.png" || relPath == "README.md" || relPath == "CHANGELOG.md":
		return path.Join("BepInEx", "plugins", modDir, relPath)
	default:
		return path.Join("BepInEx", "plugins", modDir, relPath)
	}
}

// isUnderConfig reports whether dest (relative, "/"-separated) falls under
// BepInEx/config/ — such files are never overwritten by an install/upgrade.
func isUnderConfig(dest string) bool {
	return dest == "BepInEx/config" || strings.HasPrefix(dest, "BepInEx/config/")
}

// extractPackage extracts zipPath into serverDir following the generic mod
// extraction rules (ARCHITECTURE.md §12), returning every path written,
// relative to serverDir with "/" separators. Existing BepInEx/config/*.cfg
// files are never overwritten.
func extractPackage(zipPath, serverDir, owner, name string) ([]string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open package zip: %w", err)
	}
	defer func() { _ = r.Close() }()
	if err := checkArchiveBudget(r.File, maxPackageUncompressedBytes); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(r.File))
	cleaned := make(map[string]string, len(r.File)) // original -> cleaned
	for _, f := range r.File {
		clean, err := safeZipEntryPath(f.Name)
		if err != nil {
			return nil, err
		}
		cleaned[f.Name] = clean
		if !f.FileInfo().IsDir() {
			names = append(names, clean)
		}
	}
	wrapper := commonTopFolder(names)

	var written []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel := cleaned[f.Name]
		if wrapper != "" {
			prefix := wrapper + "/"
			if !strings.HasPrefix(rel, prefix) {
				continue // stray entry outside the wrapper; ignore
			}
			rel = strings.TrimPrefix(rel, prefix)
		}
		if rel == "" {
			continue
		}

		dest := destinationFor(rel, owner, name)
		destAbs := filepath.Join(serverDir, filepath.FromSlash(dest))
		if !strings.HasPrefix(filepath.Clean(destAbs), filepath.Clean(serverDir)+string(filepath.Separator)) {
			return nil, fmt.Errorf("zip-slip: computed destination escapes server dir: %q", dest)
		}

		if isUnderConfig(dest) && fileExists(destAbs) {
			continue // never overwrite an existing cfg
		}

		if err := extractOne(f, destAbs); err != nil {
			return nil, fmt.Errorf("extract %s: %w", f.Name, err)
		}
		written = append(written, dest)
	}
	sort.Strings(written)
	return written, nil
}

func extractOne(f *zip.File, destAbs string) error {
	if err := os.MkdirAll(filepath.Dir(destAbs), 0o750); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	// Never trust archive modes beyond "executable or not": no setuid bits,
	// nothing group- or world-writable.
	perm := os.FileMode(0o640)
	if f.Mode().Perm()&0o100 != 0 {
		perm = 0o750
	}
	out, err := os.OpenFile(destAbs, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm) //nolint:gosec // destAbs is validated against zip-slip and joined under the instance server dir
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	// A zip entry's real size is unknown until inflated: bound it by the size
	// the header declares (plus one byte to detect a lie) so a small archive
	// cannot expand into an unbounded write.
	if err := copyDeclared(out, rc, f.UncompressedSize64); err != nil {
		return fmt.Errorf("%s: %w", f.Name, err)
	}
	return nil
}

// maxPackageUncompressedBytes caps the total declared size of one package.
const maxPackageUncompressedBytes = 2 << 30

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

// copyDeclared copies at most declared bytes from src and fails if src holds
// more, which means the zip header lied about the entry's size.
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

// removeManagedFiles deletes files (relative to serverDir), skipping any
// *.cfg (never delete a config file, even one this mod recorded owning a
// long time ago — ARCHITECTURE.md §12), then removes any now-empty parent
// directories under BepInEx/.
func removeManagedFiles(serverDir string, files []string) error {
	bepinexDir := filepath.Join(serverDir, "BepInEx")
	dirs := map[string]struct{}{}
	for _, rel := range files {
		if strings.HasSuffix(strings.ToLower(rel), ".cfg") {
			continue
		}
		abs := filepath.Join(serverDir, filepath.FromSlash(rel))
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", rel, err)
		}
		dirs[filepath.Dir(abs)] = struct{}{}
	}
	for dir := range dirs {
		removeEmptyDirsUpTo(dir, bepinexDir)
	}
	return nil
}

// removeEmptyDirsUpTo removes dir and each now-empty ancestor, stopping at
// (and never removing) boundary itself.
func removeEmptyDirsUpTo(dir, boundary string) {
	boundary = filepath.Clean(boundary)
	for {
		dir = filepath.Clean(dir)
		if dir == boundary || !strings.HasPrefix(dir, boundary+string(filepath.Separator)) {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// setFilesEnabled renames every managed *.dll <-> *.dll.disabled to match
// enabled, and returns the updated file list.
func setFilesEnabled(serverDir string, files []string, enabled bool) ([]string, error) {
	out := make([]string, len(files))
	for i, rel := range files {
		lower := strings.ToLower(rel)
		switch {
		case enabled && strings.HasSuffix(lower, ".dll.disabled"):
			newRel := rel[:len(rel)-len(".disabled")]
			if err := renameManaged(serverDir, rel, newRel); err != nil {
				return nil, err
			}
			out[i] = newRel
		case !enabled && strings.HasSuffix(lower, ".dll"):
			newRel := rel + ".disabled"
			if err := renameManaged(serverDir, rel, newRel); err != nil {
				return nil, err
			}
			out[i] = newRel
		default:
			out[i] = rel
		}
	}
	return out, nil
}

func renameManaged(serverDir, from, to string) error {
	fromAbs := filepath.Join(serverDir, filepath.FromSlash(from))
	toAbs := filepath.Join(serverDir, filepath.FromSlash(to))
	if !fileExists(fromAbs) {
		return nil // already in the desired state on disk; keep the record in sync
	}
	if err := os.Rename(fromAbs, toAbs); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", from, to, err)
	}
	return nil
}

// manifestFile is the subset of a Thunderstore package manifest.json this
// package cares about, for manual zip uploads.
type manifestFile struct {
	Name         string   `json:"name"`
	VersionNum   string   `json:"version_number"`
	WebsiteURL   string   `json:"website_url"`
	Description  string   `json:"description"`
	Dependencies []string `json:"dependencies"`
	Owner        string   `json:"owner"` // not part of the Thunderstore spec but some tools add it
	Author       string   `json:"author"`
}

// manualZipIdentity reads zipPath's manifest.json and derives the mod
// identity an operator-uploaded zip should be recorded under: owner is
// "local" unless the manifest carries an owner/author-like field. It does
// not extract anything; call extractPackage separately once any previous
// version's stale files have been reconciled.
func manualZipIdentity(zipPath string) (owner, name, version string, deps []string, err error) {
	mf, err := readManifest(zipPath)
	if err != nil {
		return "", "", "", nil, err
	}
	owner = "local"
	if o := strings.TrimSpace(mf.Owner); o != "" {
		owner = sanitizeSlug(o)
	} else if a := strings.TrimSpace(mf.Author); a != "" {
		owner = sanitizeSlug(a)
	}
	name = sanitizeSlug(mf.Name)
	if name == "" {
		return "", "", "", nil, domain.E(domain.CodeValidationFailed, "manifest.json is missing a name")
	}
	version = mf.VersionNum
	if version == "" {
		version = "0.0.0"
	}
	return owner, name, version, mf.Dependencies, nil
}

func readManifest(zipPath string) (manifestFile, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return manifestFile{}, fmt.Errorf("open upload zip: %w", err)
	}
	defer func() { _ = r.Close() }()
	if err := checkArchiveBudget(r.File, maxPackageUncompressedBytes); err != nil {
		return manifestFile{}, err
	}

	names := make([]string, 0, len(r.File))
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			if clean, err := safeZipEntryPath(f.Name); err == nil {
				names = append(names, clean)
			}
		}
	}
	wrapper := commonTopFolder(names)
	want := "manifest.json"
	if wrapper != "" {
		want = wrapper + "/manifest.json"
	}
	for _, f := range r.File {
		clean, err := safeZipEntryPath(f.Name)
		if err != nil {
			return manifestFile{}, err
		}
		if clean != want {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return manifestFile{}, fmt.Errorf("open manifest.json: %w", err)
		}
		defer func() { _ = rc.Close() }()
		data, err := io.ReadAll(rc)
		if err != nil {
			return manifestFile{}, fmt.Errorf("read manifest.json: %w", err)
		}
		var mf manifestFile
		if err := json.Unmarshal(data, &mf); err != nil {
			return manifestFile{}, domain.Wrap(domain.CodeValidationFailed, "invalid manifest.json", err)
		}
		return mf, nil
	}
	return manifestFile{}, domain.E(domain.CodeValidationFailed, "zip does not contain a manifest.json")
}

// sanitizeSlug lower-cases s and replaces anything not [A-Za-z0-9._-] with
// "_", producing a safe directory-name component.
func sanitizeSlug(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// installSingleDLL copies an uploaded .dll to BepInEx/plugins/local-<stem>/
// and returns the mod identity plus the single recorded file path.
func installSingleDLL(dllPath, serverDir, originalFilename string) (owner, name string, files []string, err error) {
	stem := strings.TrimSuffix(filepath.Base(originalFilename), filepath.Ext(originalFilename))
	stem = sanitizeSlug(stem)
	if stem == "" {
		return "", "", nil, domain.E(domain.CodeValidationFailed, "invalid dll filename")
	}
	owner = "local"
	name = stem
	destRel := path.Join("BepInEx", "plugins", "local-"+stem, stem+".dll")
	destAbs := filepath.Join(serverDir, filepath.FromSlash(destRel))
	if err := os.MkdirAll(filepath.Dir(destAbs), 0o750); err != nil {
		return "", "", nil, fmt.Errorf("create plugin dir: %w", err)
	}
	src, err := os.Open(dllPath) //nolint:gosec // dllPath is a manager-owned temp file created from the operator's upload
	if err != nil {
		return "", "", nil, fmt.Errorf("open uploaded dll: %w", err)
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(destAbs, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640) //nolint:gosec // destAbs is built from a sanitized slug under the instance server dir
	if err != nil {
		return "", "", nil, fmt.Errorf("create plugin file: %w", err)
	}
	defer func() { _ = dst.Close() }()
	if _, err := io.Copy(dst, src); err != nil {
		return "", "", nil, fmt.Errorf("copy uploaded dll: %w", err)
	}
	return owner, name, []string{destRel}, nil
}
