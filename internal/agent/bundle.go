package agent

import (
	"bufio"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/selfupdate"
)

//go:embed assets/*
var assets embed.FS

var semverTag = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

// maxBundleBytes bounds the plugin package download.
const maxBundleBytes = 32 << 20

// ReleaseSource looks up GitHub releases (implemented by selfupdate.Client).
type ReleaseSource interface {
	ByTag(ctx context.Context, tag string) (*selfupdate.Release, error)
	Latest(ctx context.Context) (*selfupdate.Release, error)
}

// Bundle resolves the agent package (valheim-ui-agent.zip) for this manager
// build, in order: an override path in VALHEIM_UI_AGENT_ZIP (development),
// the copy embedded into release binaries, then the asset of the GitHub
// release matching the manager's version (or the latest release for
// untagged builds), verified against the release's SHA256SUMS.
type Bundle struct {
	version  string // manager version tag, e.g. v1.5.0, or "dev"
	cacheDir string
	releases ReleaseSource
	http     *http.Client
	override string
}

// NewBundle builds a Bundle for the manager running version (as reported by
// `valheim-ui version`), caching downloads under cacheDir.
func NewBundle(version, cacheDir string, releases ReleaseSource, hc *http.Client) *Bundle {
	if hc == nil {
		hc = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Bundle{version: version, cacheDir: cacheDir, releases: releases, http: hc, override: os.Getenv("VALHEIM_UI_AGENT_ZIP")}
}

// Version is the plugin version this bundle provides: the manager's own
// version without the "v", or "0.0.0" for development builds.
func (b *Bundle) Version() string {
	if semverTag.MatchString(b.version) {
		return strings.TrimPrefix(b.version, "v")
	}
	return "0.0.0"
}

// Fetch returns a local path to the plugin package.
func (b *Bundle) Fetch(ctx context.Context) (string, error) {
	if b.override != "" {
		if fi, err := os.Stat(b.override); err == nil && fi.Mode().IsRegular() {
			return b.override, nil
		}
		return "", fmt.Errorf("agent: VALHEIM_UI_AGENT_ZIP=%s is not a file", b.override)
	}
	if p, ok, err := b.embedded(); err != nil {
		return "", err
	} else if ok {
		return p, nil
	}
	return b.fromRelease(ctx)
}

// embedded materialises the zip compiled into a release binary, if any.
func (b *Bundle) embedded() (string, bool, error) {
	data, err := fs.ReadFile(assets, "assets/"+domain.AgentAssetName)
	if err != nil || len(data) == 0 {
		return "", false, nil
	}
	dir := filepath.Join(b.cacheDir, "agent", "embedded-"+b.Version())
	path := filepath.Join(dir, domain.AgentAssetName)
	if fi, err := os.Stat(path); err == nil && fi.Size() == int64(len(data)) {
		return path, true, nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", false, fmt.Errorf("agent: cache dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o640); err != nil { //nolint:gosec // cache file, not a secret
		return "", false, fmt.Errorf("agent: write embedded package: %w", err)
	}
	return path, true, nil
}

// fromRelease downloads and verifies the asset from GitHub.
func (b *Bundle) fromRelease(ctx context.Context) (string, error) {
	if b.releases == nil {
		return "", domain.E(domain.CodeUpstreamError, "no agent package is bundled with this build and no release source is configured")
	}
	var rel *selfupdate.Release
	var err error
	if semverTag.MatchString(b.version) {
		rel, err = b.releases.ByTag(ctx, b.version)
	} else {
		rel, err = b.releases.Latest(ctx)
	}
	if err != nil {
		return "", err
	}
	if rel == nil {
		return "", domain.Ef(domain.CodeUpstreamError, "no release found for %s", b.version)
	}
	dir := filepath.Join(b.cacheDir, "agent", rel.Tag)
	path := filepath.Join(dir, domain.AgentAssetName)
	if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
		return path, nil
	}
	var zipURL, sumsURL string
	var zipSize int64
	for _, a := range rel.Assets {
		switch a.Name {
		case domain.AgentAssetName:
			zipURL, zipSize = a.DownloadURL, a.Size
		case "SHA256SUMS":
			sumsURL = a.DownloadURL
		}
	}
	if zipURL == "" {
		return "", domain.Ef(domain.CodeUpstreamError, "release %s has no %s (releases before v1.5.0 ship no agent)", rel.Tag, domain.AgentAssetName)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("agent: cache dir: %w", err)
	}
	tmp := path + ".part"
	if err := b.download(ctx, zipURL, tmp, zipSize); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if sumsURL != "" {
		want, err := b.expectedSum(ctx, sumsURL)
		if err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
		got, err := fileSHA256(tmp)
		if err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
		if want != "" && got != want {
			_ = os.Remove(tmp)
			return "", domain.Ef(domain.CodeUpstreamError, "%s checksum mismatch: expected %s, got %s", domain.AgentAssetName, want, got)
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("agent: install package: %w", err)
	}
	return path, nil
}

func (b *Bundle) download(ctx context.Context, url, dest string, size int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("agent: build download: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := b.http.Do(req)
	if err != nil {
		return domain.Wrap(domain.CodeUpstreamError, "download "+domain.AgentAssetName, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return domain.Ef(domain.CodeUpstreamError, "download %s: %s", domain.AgentAssetName, resp.Status)
	}
	limit := int64(maxBundleBytes)
	if size > 0 && size < limit {
		limit = size + 1
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640) //nolint:gosec // cache file
	if err != nil {
		return fmt.Errorf("agent: create download: %w", err)
	}
	n, copyErr := io.Copy(f, io.LimitReader(resp.Body, limit))
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("agent: write download: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("agent: close download: %w", closeErr)
	}
	if size > 0 && n != size {
		return domain.Ef(domain.CodeUpstreamError, "%s size mismatch: expected %d bytes, got %d", domain.AgentAssetName, size, n)
	}
	return nil
}

func (b *Bundle) expectedSum(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("agent: build checksum request: %w", err)
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return "", domain.Wrap(domain.CodeUpstreamError, "download SHA256SUMS", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", domain.Ef(domain.CodeUpstreamError, "download SHA256SUMS: %s", resp.Status)
	}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 64<<10))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == domain.AgentAssetName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ErrNotBundled is returned by Fetch when no source can provide the package.
var ErrNotBundled = errors.New("agent package unavailable")
