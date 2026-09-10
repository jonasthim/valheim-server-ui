//go:build !windows

// Linux-only parts of baseEnv: LD_LIBRARY_PATH and the HOME pin.

package launcher

import (
	"strings"
	"testing"
)

func TestBaseEnvPinsHome(t *testing.T) {
	env := baseEnv([]string{"HOME=/home/valheim", "PATH=/usr/bin"}, "/var/lib/valheim")
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "HOME=/var/lib/valheim") || strings.Contains(joined, "HOME=/home/valheim") {
		t.Fatalf("HOME not pinned: %v", env)
	}
	env = baseEnv([]string{"PATH=/usr/bin"}, "/var/lib/valheim")
	if !strings.Contains(strings.Join(env, "\n"), "HOME=/var/lib/valheim") {
		t.Fatalf("HOME not added when absent: %v", env)
	}
	env = baseEnv([]string{"HOME=/home/dev"}, "")
	if !strings.Contains(strings.Join(env, "\n"), "HOME=/home/dev") {
		t.Fatalf("empty home must leave HOME untouched: %v", env)
	}
}

func TestBaseEnv(t *testing.T) {
	got := baseEnv([]string{"FOO=bar", "LD_LIBRARY_PATH=/opt/lib"}, "")
	want := map[string]string{
		"FOO":             "bar",
		"LD_LIBRARY_PATH": "./linux64:/opt/lib",
		"SteamAppId":      "892970",
	}
	assertEnvEquals(t, got, want)

	got2 := baseEnv(nil, "")
	want2 := map[string]string{
		"LD_LIBRARY_PATH": "./linux64:",
		"SteamAppId":      "892970",
	}
	assertEnvEquals(t, got2, want2)
}
