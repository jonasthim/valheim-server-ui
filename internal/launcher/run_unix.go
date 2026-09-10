//go:build !windows

package launcher

import (
	"syscall"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// platformExec replaces the launcher process with the game server.
var platformExec execFunc = syscall.Exec

// addPlatformEnv pins HOME and prepends the game's bundled library dir.
func addPlatformEnv(setEnv func(key, value string), env map[string]string, home string) {
	if home != "" {
		setEnv("HOME", home)
	}
	setEnv("LD_LIBRARY_PATH", "./linux64:"+env["LD_LIBRARY_PATH"])
}

// platformDoorstop: on Linux BepInEx is enabled purely through the doorstop
// environment (LD_PRELOAD), already applied by applyBepInExEnv, so the game
// arguments are passed through unchanged.
func platformDoorstop(launch domain.Launch, env []string) ([]string, []string) {
	return env, launch.Args
}
