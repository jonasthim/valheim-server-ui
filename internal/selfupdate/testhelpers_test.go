package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// fakeBinaryScript is a tiny valheim-ui stand-in: it prints
// "valheim-ui <tag> (test)" (matching main.go's real `version` output shape)
// when invoked as `<binary> version`, so Upgrader.Apply's sanity check
// passes against it.
func fakeBinaryScript(tag string) string {
	return "#!/bin/sh\nif [ \"$1\" = \"version\" ]; then\n  echo \"valheim-ui " + tag + " (test)\"\nfi\nexit 0\n"
}

// buildReleaseAssets builds the two files a real release publishes: the
// tarball (containing a single fake "valheim-ui" executable) and a matching
// SHA256SUMS.
func buildReleaseAssets(t *testing.T, tag string) (tarball, sums []byte) {
	t.Helper()
	script := fakeBinaryScript(tag)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: binaryName, Mode: 0o755, Size: int64(len(script))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write([]byte(script)); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	tarball = buf.Bytes()

	sum := sha256.Sum256(tarball)
	sums = []byte(hex.EncodeToString(sum[:]) + "  " + archiveName + "\n")
	return tarball, sums
}

// githubServerOptions configures newGitHubServer's fake GitHub API.
type githubServerOptions struct {
	tag           string
	tarball, sums []byte
	corruptTar    bool          // serve mismatched bytes for the tarball (checksum test)
	assetDelay    time.Duration // sleep before serving each asset (widens a race window in tests)
}

// newGitHubServer starts an httptest server implementing just enough of the
// GitHub releases API for Client: GET /repos/<repo>/releases/latest and
// .../releases/tags/<tag> (404 for any other tag), and the two asset
// downloads.
func newGitHubServer(t *testing.T, opt githubServerOptions) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server

	writeRelease := func(w http.ResponseWriter, tag string) {
		body := map[string]any{
			"tag_name":     tag,
			"name":         tag,
			"body":         "release notes for " + tag,
			"html_url":     "https://github.com/" + domain.GitHubRepo + "/releases/tag/" + tag,
			"published_at": time.Now().UTC().Format(time.RFC3339),
			"assets": []map[string]any{
				{"name": archiveName, "browser_download_url": srv.URL + "/assets/" + archiveName, "size": len(opt.tarball)},
				{"name": sumsName, "browser_download_url": srv.URL + "/assets/" + sumsName, "size": len(opt.sums)},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("encode fake release: %v", err)
		}
	}

	prefix := "/repos/" + domain.GitHubRepo + "/releases/"
	mux.HandleFunc(prefix+"latest", func(w http.ResponseWriter, _ *http.Request) {
		writeRelease(w, opt.tag)
	})
	mux.HandleFunc(prefix+"tags/", func(w http.ResponseWriter, r *http.Request) {
		tag := strings.TrimPrefix(r.URL.Path, prefix+"tags/")
		if tag != opt.tag {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeRelease(w, tag)
	})
	mux.HandleFunc("/assets/"+archiveName, func(w http.ResponseWriter, _ *http.Request) {
		if opt.assetDelay > 0 {
			time.Sleep(opt.assetDelay)
		}
		data := opt.tarball
		if opt.corruptTar {
			// Flip a byte without changing the length, so the download
			// still matches the advertised asset size and only the
			// checksum verification catches the corruption.
			data = append([]byte(nil), data...)
			data[0] ^= 0xff
		}
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/assets/"+sumsName, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(opt.sums)
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newNotFoundServer simulates a repository with no releases published yet:
// every releases endpoint returns 404.
func newNotFoundServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// runVersion runs `<path> version` and returns its combined output, for
// tests to assert which release ended up installed.
func runVersion(t *testing.T, path string) (string, error) {
	t.Helper()
	out, err := exec.Command(path, "version").CombinedOutput() //nolint:gosec // test fixture binary
	return string(out), err
}

// newForbiddenServer simulates GitHub API rate limiting.
func newForbiddenServer(t *testing.T, message string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(message))
	}))
	t.Cleanup(srv.Close)
	return srv
}
