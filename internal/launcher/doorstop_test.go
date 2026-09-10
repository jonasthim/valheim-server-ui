package launcher

import (
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func domainLaunch(serverDir string, bepinex bool, args []string) domain.Launch {
	return domain.Launch{ServerDir: serverDir, LogDir: serverDir, Args: args, BepInEx: bepinex}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
