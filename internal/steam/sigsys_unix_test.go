//go:build !windows

package steam

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A child killed by SIGSYS is what a systemd SystemCallArchitectures filter
// does to the 32-bit steamcmd; the error must say so and name the remedy,
// not just "signal: bad system call".
func TestRunCommandEnv_SigsysIsExplained(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "steamcmd.sh")
	if err := os.WriteFile(script, []byte("#!/bin/bash\nkill -s SYS $$\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := runCommandEnv(context.Background(), script, nil, &out, runOptions{})
	if err == nil {
		t.Fatal("expected an error from a child killed by SIGSYS")
	}
	for _, want := range []string{"SIGSYS", "SystemCallArchitectures", "install.sh"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}
