// Package selfupdate implements the manager's own self-upgrade (WP-30, see
// docs/ARCHITECTURE.md §9/§16): a GitHub releases client, semantic version
// comparison, a periodic checker that publishes domain.EventAppUpdateAvailable
// and can trigger an automatic upgrade when nobody is playing, and an
// Upgrader that downloads, verifies and atomically swaps the running binary.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// defaultAPIBase is the real GitHub REST API. Tests and wiring can point a
// Client elsewhere with WithBaseURL.
const defaultAPIBase = "https://api.github.com"

// Asset is one downloadable file attached to a release.
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

// Release is the subset of the GitHub release representation this package
// needs. Tags are vX.Y.Z (see domain.GitHubRepo / ARCHITECTURE.md §16).
type Release struct {
	Tag         string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

// ClientOption configures a Client at construction time.
type ClientOption func(*Client)

// WithBaseURL overrides the GitHub API base URL (tests point this at an
// httptest server).
func WithBaseURL(base string) ClientOption {
	return func(c *Client) { c.base = strings.TrimRight(base, "/") }
}

// WithHTTPClient overrides the default http.Client (e.g. for a custom
// transport/timeout in tests).
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) {
		if hc != nil {
			c.http = hc
		}
	}
}

// Client talks to the GitHub releases API for one repository.
type Client struct {
	repo    string
	version string // included in the User-Agent header
	base    string
	http    *http.Client
}

// NewClient builds a Client for repo (owner/name, e.g. domain.GitHubRepo).
// version is the manager's own running version, sent as part of the
// User-Agent header (GitHub asks API consumers to identify themselves).
func NewClient(repo, version string, opts ...ClientOption) *Client {
	c := &Client{repo: repo, version: version, base: defaultAPIBase, http: http.DefaultClient}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Latest fetches the most recent published release. A repository with no
// releases yet (GitHub returns 404) is reported as (nil, nil): the checker
// treats "no release" as nothing to do, not a failure.
func (c *Client) Latest(ctx context.Context) (*Release, error) {
	return c.get(ctx, fmt.Sprintf("%s/repos/%s/releases/latest", c.base, c.repo), true)
}

// ByTag fetches the release tagged tag (e.g. "v1.2.0"). Unlike Latest, a
// missing tag is reported as (nil, nil) too: callers (the upgrade service,
// the CLI) turn that into their own "release not found" error with the
// requested tag in the message.
func (c *Client) ByTag(ctx context.Context, tag string) (*Release, error) {
	return c.get(ctx, fmt.Sprintf("%s/repos/%s/releases/tags/%s", c.base, c.repo, url.PathEscape(tag)), true)
}

func (c *Client) get(ctx context.Context, url string, allow404 bool) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "valheim-server-ui/"+c.version)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, domain.Wrap(domain.CodeUpstreamError, "github releases request", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound && allow404 {
		return nil, nil
	}
	if resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return nil, domain.Ef(domain.CodeUpstreamError, "github rate limit or forbidden: %s", msg)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, domain.Ef(domain.CodeUpstreamError, "github releases request failed: %s", resp.Status)
	}

	var rel Release
	// A release document is a few KB; bound it so a huge body (or a huge
	// release-notes field) cannot be held in memory or re-served to every
	// dashboard poll.
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseDocBytes)).Decode(&rel); err != nil {
		return nil, domain.Wrap(domain.CodeUpstreamError, "decode github release", err)
	}
	return &rel, nil
}

// maxReleaseDocBytes bounds one GitHub release JSON document.
const maxReleaseDocBytes = 1 << 20
