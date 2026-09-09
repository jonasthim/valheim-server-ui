package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// In privileged mode the Upgrader never touches the binary directory: it
// stages the verified file and asks unitctl to install it.
func TestUpgrader_ApplyPrivilegedStagesAndCallsUnitctl(t *testing.T) {
	binDir := t.TempDir()
	oldPath := installedBinary(t, binDir, "v1.0.0")
	staging := filepath.Join(t.TempDir(), "staging")

	tarball, sums := buildReleaseAssets(t, "v1.1.0")
	srv := newGitHubServer(t, githubServerOptions{tag: "v1.1.0", tarball: tarball, sums: sums})
	client := NewClient(domain.GitHubRepo, "v1.0.0", WithBaseURL(srv.URL))
	rel, err := client.Latest(context.Background())
	if err != nil || rel == nil {
		t.Fatalf("fetch fake release: %v", err)
	}

	up := NewUpgrader(oldPath, "v1.0.0", srv.Client())
	up.SetStagingDir(staging)
	up.SetPrivileged("/usr/local/lib/valheim-ui/unitctl", func() bool { return true })
	var gotArgs []string
	up.runUnitctl = func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		staged := filepath.Join(staging, stagedBinaryName)
		fi, err := os.Stat(staged)
		if err != nil {
			return nil, err
		}
		if fi.Mode().Perm()&0o100 == 0 {
			return nil, errors.New("staged binary is not executable")
		}
		// What the real wrapper does after verifying the checksum.
		if err := os.Rename(oldPath, oldPath+".prev"); err != nil {
			return nil, err
		}
		if err := os.Rename(staged, oldPath); err != nil {
			return nil, err
		}
		return []byte("installed v1.1.0"), nil
	}

	var log bytes.Buffer
	prevPath, err := up.Apply(context.Background(), rel, &log)
	if err != nil {
		t.Fatalf("Apply: %v (log: %s)", err, log.String())
	}
	if strings.Join(gotArgs, " ") != "apply-upgrade v1.1.0" {
		t.Fatalf("unitctl args = %v, want [apply-upgrade v1.1.0]", gotArgs)
	}
	if prevPath != oldPath+".prev" {
		t.Fatalf("prevPath = %s", prevPath)
	}
	if out, err := runVersion(t, oldPath); err != nil || !strings.Contains(out, "v1.1.0") {
		t.Fatalf("installed binary reports %q (%v), want v1.1.0", out, err)
	}
	if got := readPrevVersion(oldPath, staging); got != "v1.0.0" {
		t.Fatalf("previous version recorded in staging = %q, want v1.0.0", got)
	}
	if _, err := os.Stat(filepath.Join(staging, stagedBinaryName)); !os.IsNotExist(err) {
		t.Fatalf("staged file should be consumed by the install, stat err = %v", err)
	}
}

func TestUpgrader_ApplyPrivilegedFailureLeavesBinaryAndCleansStaging(t *testing.T) {
	binDir := t.TempDir()
	oldPath := installedBinary(t, binDir, "v1.0.0")
	staging := filepath.Join(t.TempDir(), "staging")

	tarball, sums := buildReleaseAssets(t, "v1.1.0")
	srv := newGitHubServer(t, githubServerOptions{tag: "v1.1.0", tarball: tarball, sums: sums})
	client := NewClient(domain.GitHubRepo, "v1.0.0", WithBaseURL(srv.URL))
	rel, _ := client.Latest(context.Background())

	up := NewUpgrader(oldPath, "v1.0.0", srv.Client())
	up.SetStagingDir(staging)
	up.SetPrivileged("/x/unitctl", func() bool { return true })
	up.runUnitctl = func(context.Context, ...string) ([]byte, error) {
		return []byte("unitctl: staged binary does not match the checksum published for v1.1.0"), errors.New("exit status 1")
	}

	var log bytes.Buffer
	if _, err := up.Apply(context.Background(), rel, &log); err == nil {
		t.Fatal("expected Apply to fail when unitctl refuses")
	}
	if !strings.Contains(log.String(), "does not match") {
		t.Fatalf("unitctl output should be in the job log, got %q", log.String())
	}
	if out, err := runVersion(t, oldPath); err != nil || !strings.Contains(out, "v1.0.0") {
		t.Fatalf("current binary must be untouched, reports %q (%v)", out, err)
	}
	if _, err := os.Stat(filepath.Join(staging, stagedBinaryName)); !os.IsNotExist(err) {
		t.Fatalf("staged file should be removed after a failed install, stat err = %v", err)
	}
}

func TestCanSelfUpgradeWithPrivilegedMode(t *testing.T) {
	binDir := t.TempDir()
	exe := installedBinary(t, binDir, "v1.0.0")
	if err := os.Chmod(binDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(binDir, 0o755) })
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	if ok, reason := canSelfUpgradeWith(exe, "v1.0.0", "", false); ok {
		t.Fatalf("read-only bin dir must block rename mode, got ok (reason %q)", reason)
	}
	staging := filepath.Join(t.TempDir(), "staging")
	if ok, reason := canSelfUpgradeWith(exe, "v1.0.0", staging, true); !ok {
		t.Fatalf("privileged mode only needs a writable staging dir, got %q", reason)
	}
	if ok, _ := canSelfUpgradeWith(exe, "v1.0.0", "", true); ok {
		t.Fatal("privileged mode without a staging dir must report unavailable")
	}
}
