//go:build !windows

// HOME pinning and the script-based runner are unix-only concerns.

package steam

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression for the first production deployment: steamcmd inherited the
// valheim account's passwd HOME (/home/valheim) while the systemd unit denies
// /home, so Valve reported "Disk write failure". HOME must be pinned to the
// data directory for every invocation.
func TestRunCommandPinsHome(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "steamcmd.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"HOME=$HOME XDG_CONFIG_HOME=$XDG_CONFIG_HOME\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", "/home/someone-else")

	var out bytes.Buffer
	if err := runCommandEnv(context.Background(), script, nil, &out, runOptions{home: dir}); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(out.String())
	want := "HOME=" + dir + " XDG_CONFIG_HOME=" + dir + "/.config"
	if got != want {
		t.Fatalf("env not pinned: got %q want %q", got, want)
	}

	out.Reset()
	if err := runCommandEnv(context.Background(), script, nil, &out, runOptions{}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(out.String(), "HOME=/home/someone-else") {
		t.Fatalf("without WithHome the process env should be inherited, got %q", out.String())
	}
}

func TestClientWithHomeUsesPinnedRunner(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "steamcmd.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"HOME=$HOME\"\necho \"Success! App '896660' fully installed\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", "/home/someone-else")
	c := New(script, nil, WithHome(dir))
	var out bytes.Buffer
	if err := c.InstallOrUpdate(context.Background(), filepath.Join(dir, "server"), &out); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out.String(), "HOME="+dir) {
		t.Fatalf("expected pinned HOME in output, got %q", out.String())
	}
}

func TestDiskWriteFailureGetsHint(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "steamcmd.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"ERROR! Failed to install app '896660' (Disk write failure)\"\nexit 8\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := New(script, nil, WithHome(dir))
	err := c.InstallOrUpdate(context.Background(), filepath.Join(dir, "server"), nil)
	if err == nil || !strings.Contains(err.Error(), "could not write to HOME") {
		t.Fatalf("expected disk-write hint, got %v", err)
	}
}
