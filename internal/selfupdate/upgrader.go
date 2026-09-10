package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Asset/file names published by the release workflow (ARCHITECTURE.md §16).
// archiveName and binaryName are per platform (names_*.go): the Linux
// tarball holds "valheim-ui", the Windows zip holds "valheim-ui.exe".
const sumsName = "SHA256SUMS"

// maxDownloadBytes bounds a download when the release metadata does not
// report the asset's size (defence in depth against a runaway response;
// real releases always report a size, so this path is not expected to run).
const maxDownloadBytes = 200 << 20 // 200 MiB

// sanityTimeout bounds how long the freshly extracted binary is given to
// answer `<binary> version` before Apply gives up on it.
const sanityTimeout = 10 * time.Second

// Upgrader downloads, verifies and installs one GitHub release over the
// currently running binary. path is the manager binary's real (symlink-
// resolved) location, resolved once by the caller (see canSelfUpgrade /
// wire_selfupdate.go): Upgrader never follows the symlink itself, it only
// ever reads and writes path and its siblings.
type Upgrader struct {
	path    string // real path to the binary this process is running from
	version string // this process's own version, recorded for rollback
	http    *http.Client

	// stagingDir is where downloads are verified. In privileged mode it is
	// the directory unitctl reads the staged binary from; otherwise a temp
	// dir next to the binary is used.
	stagingDir string
	// privileged reports whether the root-side sudo wrapper installs the
	// binary (root-owned bin directory). When false the Upgrader renames the
	// binary into place itself (development layouts, older installs).
	privileged  func() bool
	unitctlPath string
	// runUnitctl executes the wrapper; tests inject a fake.
	runUnitctl func(ctx context.Context, args ...string) ([]byte, error)
}

// NewUpgrader builds an Upgrader for the binary at path (already resolved
// through symlinks), which is currently running currentVersion.
func NewUpgrader(path, currentVersion string, hc *http.Client) *Upgrader {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Upgrader{path: path, version: currentVersion, http: hc, privileged: func() bool { return false }}
}

// SetStagingDir chooses where verified downloads wait. It is required for
// privileged mode (unitctl reads <stagingDir>/valheim-ui.new) and optional
// otherwise.
func (u *Upgrader) SetStagingDir(dir string) { u.stagingDir = dir }

// SetPrivileged switches the install step to `sudo -n unitctl apply-upgrade
// <tag>` whenever enabled() reports true (see NewPrivilegedProbe).
func (u *Upgrader) SetPrivileged(unitctlPath string, enabled func() bool) {
	u.unitctlPath = unitctlPath
	if enabled != nil {
		u.privileged = enabled
	}
}

// Privileged reports whether the next Apply would go through unitctl.
func (u *Upgrader) Privileged() bool { return u.privileged != nil && u.privileged() }

// stagedBinaryName and prevVersionFile are the fixed names unitctl and the
// checker agree on inside the staging directory.
const (
	stagedBinaryName = "valheim-ui.new"
	prevVersionFile  = "prev.version"
	unitctlTimeout   = 3 * time.Minute
)

func (u *Upgrader) unitctl(ctx context.Context, args ...string) ([]byte, error) {
	if u.runUnitctl != nil {
		return u.runUnitctl(ctx, args...)
	}
	ctx, cancel := context.WithTimeout(ctx, unitctlTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/sudo", append([]string{"-n", u.unitctlPath}, args...)...) //nolint:gosec // fixed sudo path, fixed wrapper path from config, validated args
	return cmd.CombinedOutput()
}

// Apply downloads rel's tarball and checksum file, verifies the tarball's
// sha256, extracts the single "valheim-ui" binary, sanity-runs it, and
// atomically swaps it in: the current binary is renamed to "<path>.prev"
// (returned as prevPath) and the new one takes its place. On any failure
// after that rename, the rename is undone so the manager is never left
// without a binary. "<path>.prev.version" is written next to it with the
// version being replaced, so the UI can show AppUpdateInfo.PreviousVersion
// and Rollback knows what it is restoring.
func (u *Upgrader) Apply(ctx context.Context, rel *Release, log io.Writer) (prevPath string, err error) {
	if rel == nil {
		return "", errors.New("selfupdate: no release to apply")
	}
	if log == nil {
		log = io.Discard
	}

	privileged := u.Privileged()
	workDir := filepath.Dir(u.path)
	if u.stagingDir != "" {
		if err := os.MkdirAll(u.stagingDir, 0o750); err != nil {
			return "", fmt.Errorf("selfupdate: create staging dir %s: %w", u.stagingDir, err)
		}
		workDir = u.stagingDir
	} else if privileged {
		return "", errors.New("selfupdate: privileged mode needs a staging directory")
	}
	tmpDir, err := os.MkdirTemp(workDir, ".valheim-ui-upgrade-*")
	if err != nil {
		return "", fmt.Errorf("selfupdate: create staging dir in %s: %w", workDir, err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	_, _ = fmt.Fprintf(log, "downloading %s\n", archiveName)
	tarPath := filepath.Join(tmpDir, archiveName)
	if err := u.download(ctx, rel, archiveName, tarPath); err != nil {
		return "", err
	}

	_, _ = fmt.Fprintf(log, "downloading %s\n", sumsName)
	sumsPath := filepath.Join(tmpDir, sumsName)
	if err := u.download(ctx, rel, sumsName, sumsPath); err != nil {
		return "", err
	}

	_, _ = fmt.Fprintln(log, "verifying checksum")
	if err := verifyChecksum(tarPath, sumsPath, archiveName); err != nil {
		return "", err
	}

	_, _ = fmt.Fprintln(log, "extracting release binary")
	newPath := filepath.Join(tmpDir, binaryName)
	if err := extractSingleFile(tarPath, binaryName, newPath); err != nil {
		return "", err
	}
	if err := os.Chmod(newPath, 0o755); err != nil { //nolint:gosec // 0755 is the required executable permission for the binary
		return "", fmt.Errorf("selfupdate: chmod new binary: %w", err)
	}

	_, _ = fmt.Fprintln(log, "sanity-checking the new binary")
	if err := sanityRun(ctx, newPath, rel.Tag); err != nil {
		return "", err
	}

	prevPath = u.path + ".prev"
	if privileged {
		// Hand the verified file to the root-side wrapper, which re-checks it
		// against the release's published checksum and swaps it into the
		// root-owned bin directory.
		staged := filepath.Join(u.stagingDir, stagedBinaryName)
		if err := os.Rename(newPath, staged); err != nil {
			return "", fmt.Errorf("selfupdate: stage new binary: %w", err)
		}
		if err := os.WriteFile(filepath.Join(u.stagingDir, prevVersionFile), []byte(u.version), 0o644); err != nil { //nolint:gosec // not a secret
			_, _ = fmt.Fprintf(log, "warning: could not record previous version: %v\n", err)
		}
		_, _ = fmt.Fprintf(log, "installing %s through unitctl apply-upgrade (previous binary kept at %s)\n", rel.Tag, prevPath)
		out, err := u.unitctl(ctx, "apply-upgrade", rel.Tag)
		if msg := strings.TrimSpace(string(out)); msg != "" {
			_, _ = fmt.Fprintln(log, msg)
		}
		if err != nil {
			_ = os.Remove(staged)
			_ = os.Remove(filepath.Join(u.stagingDir, prevVersionFile))
			return "", fmt.Errorf("selfupdate: unitctl apply-upgrade: %w", err)
		}
		_, _ = fmt.Fprintf(log, "upgrade to %s installed\n", rel.Tag)
		return prevPath, nil
	}
	_, _ = fmt.Fprintf(log, "installing %s (previous binary kept at %s)\n", rel.Tag, prevPath)
	if err := os.Rename(u.path, prevPath); err != nil {
		return "", fmt.Errorf("selfupdate: move current binary to %s: %w", prevPath, err)
	}
	if err := os.Rename(newPath, u.path); err != nil {
		if rbErr := os.Rename(prevPath, u.path); rbErr != nil {
			return "", fmt.Errorf("selfupdate: install new binary: %w (restoring previous binary also failed: %v)", err, rbErr)
		}
		return "", fmt.Errorf("selfupdate: install new binary: %w", err)
	}

	verPath := prevPath + ".version"
	if err := os.WriteFile(verPath, []byte(u.version), 0o644); err != nil { //nolint:gosec // sibling of the binary, not a secret
		_, _ = fmt.Fprintf(log, "warning: could not record previous version at %s: %v\n", verPath, err)
	}

	_, _ = fmt.Fprintf(log, "upgrade to %s installed\n", rel.Tag)
	return prevPath, nil
}

// Rollback restores "<path>.prev" over the current binary (used by the
// `self-upgrade --rollback` break-glass CLI, and available to callers that
// want to undo a bad upgrade). The binary being replaced is kept as
// "<path>.rolledback" until the swap succeeds, in case the rename back
// fails partway through.
func (u *Upgrader) Rollback() error {
	if u.Privileged() {
		out, err := u.unitctl(context.Background(), "rollback-upgrade")
		if err != nil {
			return fmt.Errorf("selfupdate: unitctl rollback-upgrade: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		if u.stagingDir != "" {
			_ = os.Remove(filepath.Join(u.stagingDir, prevVersionFile))
		}
		return nil
	}
	prevPath := u.path + ".prev"
	fi, err := os.Stat(prevPath)
	if err != nil {
		return fmt.Errorf("selfupdate: no previous binary at %s: %w", prevPath, err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("selfupdate: %s is not a regular file", prevPath)
	}

	badPath := u.path + ".rolledback"
	_ = os.Remove(badPath)
	if err := os.Rename(u.path, badPath); err != nil {
		return fmt.Errorf("selfupdate: move current binary aside: %w", err)
	}
	if err := os.Rename(prevPath, u.path); err != nil {
		if rbErr := os.Rename(badPath, u.path); rbErr != nil {
			return fmt.Errorf("selfupdate: restore previous binary: %w (undo also failed: %v)", err, rbErr)
		}
		return fmt.Errorf("selfupdate: restore previous binary: %w", err)
	}
	_ = os.Remove(badPath)
	_ = os.Remove(prevPath + ".version")
	return nil
}

// download fetches the named asset from rel into destPath, bounding the
// read at the asset's reported size (or maxDownloadBytes if unreported) and
// failing if fewer bytes than reported were received.
func (u *Upgrader) download(ctx context.Context, rel *Release, name, destPath string) error {
	asset := findAsset(rel, name)
	if asset == nil {
		return domain.Ef(domain.CodeUpstreamError, "release %s is missing asset %s", rel.Tag, name)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("selfupdate: build download request for %s: %w", name, err)
	}
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := u.http.Do(req)
	if err != nil {
		return domain.Wrap(domain.CodeUpstreamError, "download "+name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return domain.Ef(domain.CodeUpstreamError, "download %s failed: %s", name, resp.Status)
	}

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) //nolint:gosec // temp staging file, not a secret
	if err != nil {
		return fmt.Errorf("selfupdate: create %s: %w", destPath, err)
	}
	defer func() { _ = out.Close() }()

	limit := asset.Size
	if limit <= 0 {
		limit = maxDownloadBytes
	} else {
		limit++ // read one extra byte so a too-large response is detectable below
	}
	n, err := io.Copy(out, io.LimitReader(resp.Body, limit))
	if err != nil {
		return fmt.Errorf("selfupdate: write %s: %w", destPath, err)
	}
	if asset.Size > 0 && n != asset.Size {
		return domain.Ef(domain.CodeUpstreamError, "download %s size mismatch: expected %d bytes, got %d", name, asset.Size, n)
	}
	return nil
}

func findAsset(rel *Release, name string) *Asset {
	for i := range rel.Assets {
		if rel.Assets[i].Name == name {
			return &rel.Assets[i]
		}
	}
	return nil
}

// verifyChecksum checks tarPath's sha256 against the line for assetName in
// the sha256sum(1)-format file at sumsPath.
func verifyChecksum(tarPath, sumsPath, assetName string) error {
	data, err := os.ReadFile(sumsPath)
	if err != nil {
		return fmt.Errorf("selfupdate: read checksums file: %w", err)
	}

	want := ""
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == assetName {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return domain.Ef(domain.CodeUpstreamError, "no checksum for %s in %s", assetName, sumsName)
	}

	f, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("selfupdate: open %s: %w", assetName, err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("selfupdate: hash %s: %w", assetName, err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return domain.Ef(domain.CodeUpstreamError, "checksum mismatch for %s: expected %s, got %s", assetName, want, got)
	}
	return nil
}

// extractSingleFile extracts the regular file named memberName from the
// archive at archivePath (.tar.gz, or .zip for the Windows release) to
// destPath. Only an exact (base-name) match is extracted, so an
// attacker-controlled archive cannot write anywhere else (no path is ever
// joined with an entry's name).
func extractSingleFile(archivePath, memberName, destPath string) error {
	if strings.HasSuffix(strings.ToLower(archivePath), ".zip") {
		return extractSingleFileZip(archivePath, memberName, destPath)
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("selfupdate: open tarball: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("selfupdate: gunzip tarball: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return domain.Ef(domain.CodeUpstreamError, "release tarball does not contain %s", memberName)
		}
		if err != nil {
			return fmt.Errorf("selfupdate: read tarball: %w", err)
		}
		if filepath.Base(filepath.Clean(hdr.Name)) != memberName || hdr.Typeflag != tar.TypeReg {
			continue
		}

		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755) //nolint:gosec // becomes the executable binary
		if err != nil {
			return fmt.Errorf("selfupdate: create extracted file: %w", err)
		}
		// hdr.Size bounds the read to exactly what the tar header declares,
		// so a truncated or padded stream cannot smuggle extra bytes in.
		if _, err := io.CopyN(out, tr, hdr.Size); err != nil {
			_ = out.Close()
			return fmt.Errorf("selfupdate: write extracted file: %w", err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("selfupdate: close extracted file: %w", err)
		}
		return nil
	}
}

// extractSingleFileZip is extractSingleFile for a zip archive.
func extractSingleFileZip(archivePath, memberName, destPath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("selfupdate: open zip: %w", err)
	}
	defer func() { _ = zr.Close() }()
	for _, zf := range zr.File {
		if filepath.Base(filepath.Clean(zf.Name)) != memberName || zf.FileInfo().IsDir() {
			continue
		}
		if zf.UncompressedSize64 > maxDownloadBytes {
			return domain.Ef(domain.CodeUpstreamError, "release zip member %s is too large", memberName)
		}
		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("selfupdate: open zip member: %w", err)
		}
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755) //nolint:gosec // becomes the executable binary
		if err != nil {
			_ = rc.Close()
			return fmt.Errorf("selfupdate: create extracted file: %w", err)
		}
		_, err = io.CopyN(out, rc, int64(zf.UncompressedSize64)) //nolint:gosec // bounded above
		_ = rc.Close()
		if err != nil {
			_ = out.Close()
			return fmt.Errorf("selfupdate: write extracted file: %w", err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("selfupdate: close extracted file: %w", err)
		}
		return nil
	}
	return domain.Ef(domain.CodeUpstreamError, "release zip does not contain %s", memberName)
}

// sanityRun runs `<path> version` with a short timeout and requires its
// output to mention tag, guarding against a corrupt or wrong-architecture
// binary before it is swapped into place.
func sanityRun(ctx context.Context, path, tag string) error {
	ctx, cancel := context.WithTimeout(ctx, sanityTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "version") //nolint:gosec // path is the freshly extracted, checksum-verified release binary
	out, err := cmd.CombinedOutput()
	if err != nil {
		return domain.Ef(domain.CodeUpstreamError, "new binary failed its sanity check: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), tag) {
		return domain.Ef(domain.CodeUpstreamError, "new binary failed its sanity check: version output does not mention %s: %s", tag, strings.TrimSpace(string(out)))
	}
	return nil
}
