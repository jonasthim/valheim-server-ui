package remote

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// uploadRclone shells out to the external rclone binary:
// `rclone copyto <zipPath> <target.Remote>/<instanceID>/<basename>`.
// Pruning a remote target is out of scope for F-1.4 (see uploadLocal's doc
// comment) -- an rclone remote grows unless pruned separately by the
// operator.
func uploadRclone(ctx context.Context, rclonePath string, target domain.BackupTarget, zipPath, instanceID string) error {
	if rclonePath == "" {
		return fmt.Errorf("rclone is not installed on the manager host")
	}

	dest := target.Remote + "/" + instanceID + "/" + filepath.Base(zipPath)
	cmd := exec.CommandContext(ctx, rclonePath, "copyto", zipPath, dest) //nolint:gosec // rclonePath is server-configured (resolved once via exec.LookPath), args are our own paths
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rclone copyto: %w: %s", err, tail(out.String(), 500))
	}
	return nil
}

// tail returns the last n bytes of s (all of s when shorter), so a failed
// upload's error carries the useful end of rclone's output without holding a
// long transcript in memory.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
