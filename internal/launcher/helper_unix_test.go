//go:build !windows

package launcher

import (
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

const exeSuffix = ""

// stopHelper ends the helper process: after exec it is the fake server, so
// SIGINT makes it exit 0 the way the direct supervisor stops a real one.
func stopHelper(t *testing.T, cmd *exec.Cmd, stdin io.WriteCloser) {
	t.Helper()
	_ = stdin.Close()
	// The fake server registers its signal handler right after writing the
	// dump; give it a moment so SIGINT hits the handler, not the default.
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil && err != os.ErrProcessDone {
		t.Fatalf("signal helper: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper process failed: %v", err)
	}
}
