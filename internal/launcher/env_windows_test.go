//go:build windows

package launcher

import (
	"strings"
	"testing"
)

// On Windows baseEnv adds only SteamAppId: no loader path, and HOME is left
// alone (the game does not read it there).
func TestBaseEnvWindows(t *testing.T) {
	env := baseEnv([]string{"HOME=C:\\Users\\x", "PATH=C:\\Windows"}, `C:\ProgramData\valheim-ui`)
	joined := strings.Join(env, "\n")
	for _, want := range []string{"HOME=C:\\Users\\x", "PATH=C:\\Windows", "SteamAppId=892970"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %v", want, env)
		}
	}
	if strings.Contains(joined, "LD_LIBRARY_PATH") || strings.Contains(joined, "ProgramData") {
		t.Fatalf("unexpected platform variables on windows: %v", env)
	}
}
