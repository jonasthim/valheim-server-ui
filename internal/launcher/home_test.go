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
