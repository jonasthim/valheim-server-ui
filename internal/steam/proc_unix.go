//go:build !windows

package steam

import (
	"os"
	"os/exec"
	"syscall"
)

// steamPlatformType is the depot flavour steamcmd downloads for the game
// server: the manager runs the server on the platform it runs on itself.
const steamPlatformType = "linux"

// setProcessGroup puts steamcmd in its own process group so killProcessGroup
// reaches the helpers it forks, not just the direct child.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

// isExecutable reports whether the steamcmd entry point can be run.
func isExecutable(_ string, info os.FileInfo) bool {
	return info.Mode()&0o111 != 0
}
