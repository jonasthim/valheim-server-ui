//go:build windows

package steam

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

const steamPlatformType = "windows"

// setProcessGroup starts steamcmd.exe in a new process group with no console
// window of its own, so a Ctrl+C aimed at the manager never reaches it.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
}

// killProcessGroup terminates steamcmd.exe. Windows has no process-group
// kill; steamcmd's own helper processes exit when their parent is gone.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

// isExecutable: Windows carries no execute bit; steamcmd is an .exe.
func isExecutable(path string, _ os.FileInfo) bool {
	return strings.EqualFold(".exe", path[max(0, len(path)-4):])
}
