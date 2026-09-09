package selfupdate

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// installedBinary writes a fake "current" binary at dir/valheim-ui, as if a
// previous release had already been installed there.
func installedBinary(t *testing.T, dir, tag string) string {
	t.Helper()
	path := filepath.Join(dir, "valheim-ui")
	if err := os.WriteFile(path, []byte(fakeBinaryScript(tag)), 0o755); err != nil {
		t.Fatalf("write installed binary: %v", err)
	}
	return path
}

func TestUpgrader_ApplyHappyPath(t *testing.T) {
	dir := t.TempDir()
	oldPath := installedBinary(t, dir, "v1.0.0")

	tarball, sums := buildReleaseAssets(t, "v1.1.0")
	srv := newGitHubServer(t, githubServerOptions{tag: "v1.1.0", tarball: tarball, sums: sums})
	client := NewClient(domain.GitHubRepo, "v1.0.0", WithBaseURL(srv.URL))
	rel, err := client.Latest(context.Background())
	if err != nil || rel == nil {
		t.Fatalf("fetch fake release: %v", err)
	}

	up := NewUpgrader(oldPath, "v1.0.0", srv.Client())
	var log bytes.Buffer
	prevPath, err := up.Apply(context.Background(), rel, &log)
	if err != nil {
		t.Fatalf("Apply: %v (log: %s)", err, log.String())
	}
	if prevPath != oldPath+".prev" {
		t.Fatalf("expected prevPath %s, got %s", oldPath+".prev", prevPath)
	}

	// The binary at oldPath is now the new v1.1.0 release.
	out, err := runVersion(t, oldPath)
	if err != nil {
		t.Fatalf("run new binary: %v", err)
	}
	if !strings.Contains(out, "v1.1.0") {
		t.Fatalf("expected new binary to report v1.1.0, got %q", out)
	}

	// The old binary is preserved at .prev.
	prevOut, err := runVersion(t, prevPath)
	if err != nil {
		t.Fatalf("run previous binary: %v", err)
	}
	if !strings.Contains(prevOut, "v1.0.0") {
		t.Fatalf("expected previous binary to report v1.0.0, got %q", prevOut)
	}

	// The previous version marker was written.
	verBytes, err := os.ReadFile(prevPath + ".version")
	if err != nil {
		t.Fatalf("read prev.version: %v", err)
	}
	if string(verBytes) != "v1.0.0" {
		t.Fatalf("expected prev.version to contain v1.0.0, got %q", verBytes)
	}

	if !strings.Contains(log.String(), "v1.1.0 installed") {
		t.Fatalf("expected log to mention the install, got %q", log.String())
	}
}

func TestUpgrader_ChecksumMismatchAborts(t *testing.T) {
	dir := t.TempDir()
	oldPath := installedBinary(t, dir, "v1.0.0")

	tarball, sums := buildReleaseAssets(t, "v1.1.0")
	srv := newGitHubServer(t, githubServerOptions{tag: "v1.1.0", tarball: tarball, sums: sums, corruptTar: true})
	client := NewClient(domain.GitHubRepo, "v1.0.0", WithBaseURL(srv.URL))
	rel, err := client.Latest(context.Background())
	if err != nil || rel == nil {
		t.Fatalf("fetch fake release: %v", err)
	}

	up := NewUpgrader(oldPath, "v1.0.0", srv.Client())
	_, err = up.Apply(context.Background(), rel, nil)
	if err == nil {
		t.Fatal("expected a checksum mismatch error")
	}
	de := domain.AsError(err)
	if !strings.Contains(de.Message, "checksum mismatch") {
		t.Fatalf("expected checksum mismatch error, got %v", err)
	}

	// The original binary must be untouched: no rename ever happened.
	if _, statErr := os.Stat(oldPath + ".prev"); !os.IsNotExist(statErr) {
		t.Fatalf("expected no .prev file after an aborted upgrade, stat err: %v", statErr)
	}
	out, err := runVersion(t, oldPath)
	if err != nil {
		t.Fatalf("run original binary: %v", err)
	}
	if !strings.Contains(out, "v1.0.0") {
		t.Fatalf("expected the original binary untouched (v1.0.0), got %q", out)
	}
}

func TestUpgrader_SanityCheckFailureRestores(t *testing.T) {
	dir := t.TempDir()
	oldPath := installedBinary(t, dir, "v1.0.0")

	// Build a tarball whose binary reports the wrong tag, so the sanity
	// check (which requires the release tag to appear in `version` output)
	// fails.
	tarball, sums := buildReleaseAssets(t, "v9.9.9-not-the-release-tag")
	srv := newGitHubServer(t, githubServerOptions{tag: "v1.1.0", tarball: tarball, sums: sums})
	client := NewClient(domain.GitHubRepo, "v1.0.0", WithBaseURL(srv.URL))
	rel, err := client.Latest(context.Background())
	if err != nil || rel == nil {
		t.Fatalf("fetch fake release: %v", err)
	}

	up := NewUpgrader(oldPath, "v1.0.0", srv.Client())
	_, err = up.Apply(context.Background(), rel, nil)
	if err == nil {
		t.Fatal("expected a sanity check failure")
	}
	de := domain.AsError(err)
	if !strings.Contains(de.Message, "sanity check") {
		t.Fatalf("expected sanity check error, got %v", err)
	}

	// Nothing was renamed: the sanity check runs before the first rename.
	if _, statErr := os.Stat(oldPath + ".prev"); !os.IsNotExist(statErr) {
		t.Fatalf("expected no .prev file after a failed sanity check, stat err: %v", statErr)
	}
	out, err := runVersion(t, oldPath)
	if err != nil {
		t.Fatalf("run original binary: %v", err)
	}
	if !strings.Contains(out, "v1.0.0") {
		t.Fatalf("expected the original binary untouched (v1.0.0), got %q", out)
	}
}

func TestUpgrader_Rollback(t *testing.T) {
	dir := t.TempDir()
	oldPath := installedBinary(t, dir, "v1.0.0")

	tarball, sums := buildReleaseAssets(t, "v1.1.0")
	srv := newGitHubServer(t, githubServerOptions{tag: "v1.1.0", tarball: tarball, sums: sums})
	client := NewClient(domain.GitHubRepo, "v1.0.0", WithBaseURL(srv.URL))
	rel, err := client.Latest(context.Background())
	if err != nil || rel == nil {
		t.Fatalf("fetch fake release: %v", err)
	}

	up := NewUpgrader(oldPath, "v1.0.0", srv.Client())
	if _, err := up.Apply(context.Background(), rel, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if err := up.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	out, err := runVersion(t, oldPath)
	if err != nil {
		t.Fatalf("run rolled-back binary: %v", err)
	}
	if !strings.Contains(out, "v1.0.0") {
		t.Fatalf("expected rollback to restore v1.0.0, got %q", out)
	}
	if _, statErr := os.Stat(oldPath + ".prev"); !os.IsNotExist(statErr) {
		t.Fatalf("expected .prev to be consumed by Rollback")
	}

	// Rolling back again with nothing left to restore is an error, not a
	// silent no-op.
	if err := up.Rollback(); err == nil {
		t.Fatal("expected Rollback to fail when there is no .prev binary")
	}
}
