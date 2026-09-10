//go:build !windows

package supervisor

import (
	"os"
	"os/exec"
	"syscall"
)

// procControl is how the direct supervisor asks a launched instance to stop.
// On Unix the launcher has exec'd the game, so signals go straight to it.
type procControl struct{}

func prepareCommand(*exec.Cmd) (*procControl, error) { return &procControl{}, nil }

func (*procControl) requestStop(proc *os.Process) error { return proc.Signal(syscall.SIGINT) }
func (*procControl) kill(proc *os.Process) error        { return proc.Signal(syscall.SIGKILL) }
func (*procControl) close()                             {}
