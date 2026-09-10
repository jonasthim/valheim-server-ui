//go:build windows

package supervisor

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

// procControl is how the direct supervisor asks a launched instance to stop.
// On Windows the launcher stays alive as a proxy in front of the game (see
// internal/launcher/run_windows.go); stop requests are one-word lines on its
// stdin, and it turns them into a console Ctrl+C or a kill.
type procControl struct {
	mu    sync.Mutex
	stdin io.WriteCloser
}

// prepareCommand gives the launcher its own hidden console (so the Ctrl+C
// it generates never reaches the manager or an operator's terminal) and a
// stdin pipe for the stop protocol.
func prepareCommand(cmd *exec.Cmd) (*procControl, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	w, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	return &procControl{stdin: w}, nil
}

func (c *procControl) send(word string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil {
		return os.ErrProcessDone
	}
	_, err := io.WriteString(c.stdin, word+"\n")
	return err
}

func (c *procControl) requestStop(proc *os.Process) error {
	if err := c.send("stop"); err != nil {
		// The proxy is not listening; nothing graceful is left to try.
		return proc.Kill()
	}
	return nil
}

func (c *procControl) kill(proc *os.Process) error {
	_ = c.send("kill")
	// Killing the proxy closes its job object, which takes the game with it.
	return proc.Kill()
}

func (c *procControl) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin != nil {
		_ = c.stdin.Close()
		c.stdin = nil
	}
}
