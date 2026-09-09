// Package mods implements api.ModService and api.ThunderstoreService:
// BepInEx install/enable, the Thunderstore package index client and cache,
// dependency resolution, package extraction/enable/disable/uninstall, and the
// BepInEx .cfg editor. See docs/ARCHITECTURE.md §12.
package mods

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// indexURL is the Thunderstore v1 package index for the Valheim community.
const indexURL = "https://thunderstore.io/c/valheim/api/v1/package/"

// downloadURLFormat builds the canonical download URL for one package
// version (ARCHITECTURE.md §12); the index's own download_url is preferred
// when present, this is the fallback.
const downloadURLFormat = "https://thunderstore.io/package/download/%s/%s/%s/"

// defaultPageSize / maxPageSize bound PackageSearch.PageSize (openapi.yaml
// /thunderstore/packages).
const (
	defaultPageSize = 50
	maxPageSize     = 100
)

// minRetryInterval is the minimum wait before retrying a failed background
// refresh, so a persistently unreachable Thunderstore does not hot-loop.
const minRetryInterval = time.Minute

// rawVersion is one entry of a package's "versions" array in the Thunderstore
// v1 index response.
type rawVersion struct {
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Description   string    `json:"description"`
	Icon          string    `json:"icon"`
	VersionNumber string    `json:"version_number"`
	Dependencies  []string  `json:"dependencies"`
	DownloadURL   string    `json:"download_url"`
	Downloads     int64     `json:"downloads"`
	DateCreated   time.Time `json:"date_created"`
	WebsiteURL    string    `json:"website_url"`
	IsActive      bool      `json:"is_active"`
	UUID4         string    `json:"uuid4"`
	FileSize      int64     `json:"file_size"`
}

// rawPackage is one entry of the Thunderstore v1 index response. versions[0]
// is always the latest.
type rawPackage struct {
	Name           string       `json:"name"`
	FullName       string       `json:"full_name"`
	Owner          string       `json:"owner"`
	PackageURL     string       `json:"package_url"`
	DateCreated    time.Time    `json:"date_created"`
	DateUpdated    time.Time    `json:"date_updated"`
	UUID4          string       `json:"uuid4"`
	RatingScore    int          `json:"rating_score"`
	IsPinned       bool         `json:"is_pinned"`
	IsDeprecated   bool         `json:"is_deprecated"`
	HasNSFWContent bool         `json:"has_nsfw_content"`
	Categories     []string     `json:"categories"`
	Versions       []rawVersion `json:"versions"`
}

// indexEntry wraps one rawPackage with precomputed, lower-cased search
// fields so Search never re-lowercases on every query.
type indexEntry struct {
	raw            rawPackage
	fullNameLower  string
	nameLower      string
	ownerLower     string
	descLower      string
	totalDownloads int64
}

// Thunderstore is a cached client for the Thunderstore Valheim package index.
// All network access goes through the injected *http.Client so tests can use
// httptest without touching the real registry.
type Thunderstore struct {
	http      *http.Client
	cacheDir  string // <data>/cache/thunderstore
	refresh   func() time.Duration
	userAgent string
	log       *slog.Logger

	mu         sync.RWMutex
	entries    []*indexEntry
	byFullName map[string]*indexEntry
	categories []string
	updatedAt  time.Time

	retryMu     sync.Mutex
	nextAttempt time.Time
}

// NewThunderstore constructs a client and loads any cached index from disk
// (cacheDir/index.json + index.ts). A missing or corrupt cache is not an
// error: the client starts empty and Refresh (called by the caller's
// background loop, see Run) populates it. refresh reports the desired
// refresh interval (e.g. from settings.thunderstore.index_refresh_hours) and
// may change at runtime.
func NewThunderstore(httpClient *http.Client, cacheDir string, refresh func() time.Duration, userAgent string, log *slog.Logger) *Thunderstore {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if refresh == nil {
		refresh = func() time.Duration { return 6 * time.Hour }
	}
	if log == nil {
		log = slog.Default()
	}
	t := &Thunderstore{
		http:       httpClient,
		cacheDir:   filepath.Join(cacheDir, "thunderstore"),
		refresh:    refresh,
		userAgent:  userAgent,
		log:        log,
		byFullName: map[string]*indexEntry{},
	}
	if err := t.loadCache(); err != nil {
		log.Warn("thunderstore: load cache", "err", err)
	}
	return t
}

func (t *Thunderstore) indexPath() string { return filepath.Join(t.cacheDir, "index.json") }
func (t *Thunderstore) tsPath() string    { return filepath.Join(t.cacheDir, "index.ts") }
func (t *Thunderstore) pkgsDir() string   { return filepath.Join(t.cacheDir, "pkgs") }

// loadCache reads a previously cached index from disk, if any.
func (t *Thunderstore) loadCache() error {
	data, err := os.ReadFile(t.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read cached index: %w", err)
	}
	var raws []rawPackage
	if err := json.Unmarshal(data, &raws); err != nil {
		return fmt.Errorf("parse cached index: %w", err)
	}
	updatedAt := t.readCachedTimestamp()
	if updatedAt.IsZero() {
		if fi, statErr := os.Stat(t.indexPath()); statErr == nil {
			updatedAt = fi.ModTime().UTC()
		}
	}
	t.setIndex(raws, updatedAt)
	return nil
}

func (t *Thunderstore) readCachedTimestamp() time.Time {
	data, err := os.ReadFile(t.tsPath())
	if err != nil {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}
	}
	return ts.UTC()
}

// Refresh downloads the index, validates it, atomically replaces the on-disk
// cache and rebuilds the in-memory index. Safe to call concurrently with
// Search/Package/etc (which read a consistent snapshot).
func (t *Thunderstore) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return fmt.Errorf("thunderstore: build request: %w", err)
	}
	req.Header.Set("User-Agent", t.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := t.http.Do(req)
	if err != nil {
		t.recordFailure()
		return domain.Wrap(domain.CodeUpstreamError, "fetch thunderstore index", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.recordFailure()
		return domain.Ef(domain.CodeUpstreamError, "thunderstore index request failed: %s", resp.Status)
	}

	body := io.Reader(resp.Body)
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			t.recordFailure()
			return domain.Wrap(domain.CodeUpstreamError, "decompress thunderstore index", err)
		}
		defer func() { _ = gz.Close() }()
		body = gz
	}

	data, err := io.ReadAll(body)
	if err != nil {
		t.recordFailure()
		return domain.Wrap(domain.CodeUpstreamError, "read thunderstore index", err)
	}

	var raws []rawPackage
	if err := json.Unmarshal(data, &raws); err != nil {
		t.recordFailure()
		return domain.Wrap(domain.CodeUpstreamError, "parse thunderstore index", err)
	}

	if err := os.MkdirAll(t.cacheDir, 0o750); err != nil {
		return fmt.Errorf("thunderstore: create cache dir: %w", err)
	}
	if err := writeFileAtomic(t.indexPath(), data, 0o640); err != nil {
		return fmt.Errorf("thunderstore: write cache: %w", err)
	}
	now := time.Now().UTC()
	if err := writeFileAtomic(t.tsPath(), []byte(now.Format(time.RFC3339)), 0o640); err != nil {
		return fmt.Errorf("thunderstore: write cache timestamp: %w", err)
	}

	t.setIndex(raws, now)
	t.retryMu.Lock()
	t.nextAttempt = time.Time{}
	t.retryMu.Unlock()
	t.log.Info("thunderstore: index refreshed", "packages", len(raws))
	return nil
}

func (t *Thunderstore) recordFailure() {
	t.retryMu.Lock()
	t.nextAttempt = time.Now().Add(minRetryInterval)
	t.retryMu.Unlock()
}

// setIndex rebuilds the in-memory index from raw package entries.
func (t *Thunderstore) setIndex(raws []rawPackage, updatedAt time.Time) {
	entries := make([]*indexEntry, 0, len(raws))
	byFullName := make(map[string]*indexEntry, len(raws))
	catSet := map[string]struct{}{}
	for i := range raws {
		raw := raws[i]
		e := &indexEntry{
			raw:           raw,
			fullNameLower: strings.ToLower(raw.FullName),
			nameLower:     strings.ToLower(raw.Name),
			ownerLower:    strings.ToLower(raw.Owner),
		}
		if len(raw.Versions) > 0 {
			e.descLower = strings.ToLower(raw.Versions[0].Description)
		}
		for _, v := range raw.Versions {
			e.totalDownloads += v.Downloads
		}
		for _, c := range raw.Categories {
			catSet[c] = struct{}{}
		}
		entries = append(entries, e)
		byFullName[strings.ToLower(raw.FullName)] = e
		byFullName[strings.ToLower(raw.Owner+"-"+raw.Name)] = e
	}
	categories := make([]string, 0, len(catSet))
	for c := range catSet {
		categories = append(categories, c)
	}
	sort.Strings(categories)

	t.mu.Lock()
	t.entries = entries
	t.byFullName = byFullName
	t.categories = categories
	t.updatedAt = updatedAt
	t.mu.Unlock()
}

// Run refreshes the index once immediately if the cache is missing or stale,
// then loops, refreshing whenever the cached index is older than
// t.refresh(), until ctx is cancelled. Intended to run on its own goroutine
// (started by wireMods).
func (t *Thunderstore) Run(ctx context.Context) {
	for {
		wait := time.Until(t.dueAt())
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if err := t.Refresh(ctx); err != nil {
			t.log.Warn("thunderstore: background refresh failed", "err", err)
		}
	}
}

func (t *Thunderstore) dueAt() time.Time {
	t.retryMu.Lock()
	retry := t.nextAttempt
	t.retryMu.Unlock()
	if !retry.IsZero() {
		return retry
	}
	t.mu.RLock()
	updated := t.updatedAt
	t.mu.RUnlock()
	if updated.IsZero() {
		return time.Now()
	}
	return updated.Add(t.refresh())
}

// IndexUpdatedAt returns the timestamp of the last successful refresh (zero
// if the index has never been loaded).
func (t *Thunderstore) IndexUpdatedAt() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.updatedAt
}

// Categories returns every distinct category across the index, sorted.
func (t *Thunderstore) Categories(_ context.Context) ([]string, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]string, len(t.categories))
	copy(out, t.categories)
	return out, nil
}

// Search implements the in-memory search described in ARCHITECTURE.md §12.
func (t *Thunderstore) Search(_ context.Context, q domain.PackageSearch) (*domain.PackageSearchResult, error) {
	t.mu.RLock()
	entries := t.entries
	updatedAt := t.updatedAt
	t.mu.RUnlock()

	query := strings.ToLower(strings.TrimSpace(q.Query))
	category := q.Category

	matched := make([]*indexEntry, 0, len(entries))
	for _, e := range entries {
		if !q.IncludeDeprecated && e.raw.IsDeprecated {
			continue
		}
		if category != "" && !containsFold(e.raw.Categories, category) {
			continue
		}
		if query != "" &&
			!strings.Contains(e.fullNameLower, query) &&
			!strings.Contains(e.nameLower, query) &&
			!strings.Contains(e.ownerLower, query) &&
			!strings.Contains(e.descLower, query) {
			continue
		}
		matched = append(matched, e)
	}

	sortEntries(matched, q.Sort)

	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	total := len(matched)
	if page > maxPage {
		page = maxPage
	}
	start := (page - 1) * pageSize
	if start < 0 || start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	out := make([]domain.PackageSummary, 0, end-start)
	for _, e := range matched[start:end] {
		out = append(out, summaryFrom(e))
	}

	return &domain.PackageSearchResult{
		Packages:       out,
		Total:          total,
		Page:           page,
		PageSize:       pageSize,
		IndexUpdatedAt: updatedAt,
	}, nil
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

func sortEntries(entries []*indexEntry, sortBy string) {
	less := func(i, j int) bool {
		a, b := entries[i], entries[j]
		switch sortBy {
		case "downloads":
			if a.totalDownloads != b.totalDownloads {
				return a.totalDownloads > b.totalDownloads
			}
		case "updated":
			if !a.raw.DateUpdated.Equal(b.raw.DateUpdated) {
				return a.raw.DateUpdated.After(b.raw.DateUpdated)
			}
		case "name":
			if a.nameLower != b.nameLower {
				return a.nameLower < b.nameLower
			}
		default: // "rating"
			if a.raw.RatingScore != b.raw.RatingScore {
				return a.raw.RatingScore > b.raw.RatingScore
			}
		}
		// Stable, deterministic tiebreaker.
		return a.fullNameLower < b.fullNameLower
	}
	sort.SliceStable(entries, less)
}

func summaryFrom(e *indexEntry) domain.PackageSummary {
	var desc, icon string
	if len(e.raw.Versions) > 0 {
		desc = e.raw.Versions[0].Description
		icon = e.raw.Versions[0].Icon
	}
	latest := ""
	if len(e.raw.Versions) > 0 {
		latest = e.raw.Versions[0].VersionNumber
	}
	return domain.PackageSummary{
		Owner:          e.raw.Owner,
		Name:           e.raw.Name,
		FullName:       e.raw.FullName,
		Description:    desc,
		IconURL:        icon,
		PackageURL:     e.raw.PackageURL,
		LatestVersion:  latest,
		RatingScore:    e.raw.RatingScore,
		TotalDownloads: e.totalDownloads,
		IsDeprecated:   e.raw.IsDeprecated,
		IsPinned:       e.raw.IsPinned,
		Categories:     append([]string{}, e.raw.Categories...),
		DateUpdated:    e.raw.DateUpdated,
	}
}

func packageFrom(e *indexEntry) *domain.Package {
	summary := summaryFrom(e)
	versions := make([]domain.PackageVersion, 0, len(e.raw.Versions))
	var website string
	for i, v := range e.raw.Versions {
		if i == 0 {
			website = v.WebsiteURL
		}
		versions = append(versions, domain.PackageVersion{
			Version:      v.VersionNumber,
			Description:  v.Description,
			DownloadURL:  v.DownloadURL,
			Dependencies: append([]string(nil), v.Dependencies...),
			Downloads:    v.Downloads,
			FileSize:     v.FileSize,
			DateCreated:  v.DateCreated,
		})
	}
	return &domain.Package{
		PackageSummary: summary,
		WebsiteURL:     website,
		Versions:       versions,
	}
}

// lookup finds an index entry by owner/name (case-insensitive).
func (t *Thunderstore) lookup(owner, name string) (*indexEntry, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	e, ok := t.byFullName[strings.ToLower(owner+"-"+name)]
	return e, ok
}

// Package returns the full detail (all versions) for owner/name.
func (t *Thunderstore) Package(_ context.Context, owner, name string) (*domain.Package, error) {
	e, ok := t.lookup(owner, name)
	if !ok {
		return nil, domain.Ef(domain.CodePackageNotFound, "package %s-%s not found", owner, name)
	}
	return packageFrom(e), nil
}

// LatestVersion returns the newest version number for owner-name, or "" if
// the package is not in the index.
func (t *Thunderstore) LatestVersion(owner, name string) (string, bool) {
	e, ok := t.lookup(owner, name)
	if !ok || len(e.raw.Versions) == 0 {
		return "", false
	}
	return e.raw.Versions[0].VersionNumber, true
}

// versionEntry returns the dependency list and download URL for one specific
// version of owner-name (used by the resolver, which must respect a pinned
// dependency version rather than always the latest).
func (t *Thunderstore) versionEntry(owner, name, version string) (rawVersion, bool) {
	e, ok := t.lookup(owner, name)
	if !ok {
		return rawVersion{}, false
	}
	for _, v := range e.raw.Versions {
		if v.VersionNumber == version {
			return v, true
		}
	}
	return rawVersion{}, false
}

// Download fetches (or reuses a cached copy of) owner/name@version's zip and
// returns its path on disk.
func (t *Thunderstore) Download(ctx context.Context, owner, name, version string) (string, error) {
	if err := validateSlug(owner); err != nil {
		return "", fmt.Errorf("owner: %w", err)
	}
	if err := validateSlug(name); err != nil {
		return "", fmt.Errorf("name: %w", err)
	}
	if version == "" {
		return "", domain.E(domain.CodeValidationFailed, "version is required")
	}
	if err := validateVersionString(version); err != nil {
		return "", fmt.Errorf("version: %w", err)
	}

	dest := filepath.Join(t.pkgsDir(), fmt.Sprintf("%s-%s-%s.zip", owner, name, version))
	if fi, err := os.Stat(dest); err == nil && fi.Size() > 0 {
		if verifyZip(dest) == nil {
			return dest, nil
		}
	}

	url := fmt.Sprintf(downloadURLFormat, owner, name, version)
	var expectedSize int64
	if v, ok := t.versionEntry(owner, name, version); ok {
		if v.DownloadURL != "" {
			url = v.DownloadURL
		}
		expectedSize = v.FileSize
	}
	// The index is data from a third party: only fetch from Thunderstore over
	// TLS, on every redirect hop, and never more than the declared size.
	if err := checkDownloadURL(url); err != nil {
		return "", err
	}

	if err := os.MkdirAll(t.pkgsDir(), 0o750); err != nil {
		return "", fmt.Errorf("create package cache dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build download request: %w", err)
	}
	req.Header.Set("User-Agent", t.userAgent)

	client := *t.http // shallow copy so the redirect policy is per download
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return checkDownloadURL(req.URL.String())
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", domain.Wrap(domain.CodeUpstreamError, "download package", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", domain.Ef(domain.CodeUpstreamError, "download %s-%s@%s failed: %s", owner, name, version, resp.Status)
	}
	limit := int64(maxPackageBytes)
	if expectedSize > 0 && expectedSize < limit {
		limit = expectedSize
	}

	tmp, err := os.CreateTemp(t.pkgsDir(), "download-*.zip.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp download file: %w", err)
	}
	tmpPath := tmp.Name()
	written, copyErr := io.Copy(tmp, io.LimitReader(resp.Body, limit+1))
	if copyErr == nil && written > limit {
		copyErr = fmt.Errorf("package larger than the allowed %d bytes", limit)
	}
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return "", domain.Wrap(domain.CodeUpstreamError, "save downloaded package", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("close temp download file: %w", closeErr)
	}
	if err := verifyZip(tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", domain.Wrap(domain.CodeUpstreamError, "downloaded package is not a valid zip", err)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("finalize downloaded package: %w", err)
	}
	return dest, nil
}

// maxPackageBytes caps a single Thunderstore package download when the index
// does not declare a size (Valheim packs are a few hundred MB at most).
const maxPackageBytes = 512 << 20

// thunderstoreHosts are the only hosts a package may be fetched from.
func isThunderstoreHost(host string) bool {
	host = strings.ToLower(host)
	return host == "thunderstore.io" || strings.HasSuffix(host, ".thunderstore.io")
}

// checkDownloadURL enforces https + a Thunderstore host for package downloads
// (including every redirect hop), so a poisoned index cannot make the manager
// fetch from an arbitrary address.
func checkDownloadURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return domain.Ef(domain.CodeUpstreamError, "invalid package download URL %q", raw)
	}
	if u.Scheme != "https" || !isThunderstoreHost(u.Hostname()) {
		return domain.Ef(domain.CodeUpstreamError, "refusing package download from %q: only https://thunderstore.io is allowed", u.Host)
	}
	return nil
}

// validateVersionString accepts Thunderstore version numbers (digits and
// dots, optionally a short suffix) so a version can never shape a cache path.
func validateVersionString(v string) error {
	if len(v) > 64 {
		return domain.E(domain.CodeValidationFailed, "too long")
	}
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '.', r == '-', r == '_':
		default:
			return domain.E(domain.CodeValidationFailed, "must contain only letters, digits, dots, dashes and underscores")
		}
	}
	return nil
}

// maxPage bounds the page number so (page-1)*pageSize cannot overflow.
const maxPage = 1_000_000
