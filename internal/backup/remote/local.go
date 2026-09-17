package remote

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// uploadLocal copies zipPath to <target.Path>/<instanceID>/<basename>,
// creating the destination directory if needed, then (when target.KeepLast >
// 0) deletes the oldest *.zip files in that directory beyond KeepLast.
// Pruning a remote (rclone) target is explicitly out of scope for F-1.4: an
// rclone remote is expected to be pruned by the operator or a separate
// rclone-side policy, never by this manager.
func uploadLocal(target domain.BackupTarget, zipPath, instanceID string) error {
	destDir := filepath.Join(target.Path, instanceID)
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("create local backup target directory: %w", err)
	}

	dest := filepath.Join(destDir, filepath.Base(zipPath))
	if err := copyFileAtomic(zipPath, dest); err != nil {
		return err
	}

	if target.KeepLast > 0 {
		if err := pruneOldestZips(destDir, target.KeepLast); err != nil {
			return fmt.Errorf("prune local backup target directory: %w", err)
		}
	}
	return nil
}

// copyFileAtomic copies src to a temp file in dest's directory, then renames
// it into place, so a reader of dest never observes a partially written file.
func copyFileAtomic(src, dest string) (err error) {
	in, err := os.Open(src) //nolint:gosec // src is our own backup zip path, not user input
	if err != nil {
		return fmt.Errorf("open backup zip: %w", err)
	}
	defer func() { _ = in.Close() }()

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".upload-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = io.Copy(tmp, in); err != nil { //nolint:gosec // backup zip, no smaller bound is meaningful here
		_ = tmp.Close()
		return fmt.Errorf("copy backup zip: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err = os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}
	return nil
}

// pruneOldestZips deletes every *.zip in dir beyond the keepLast newest (by
// modification time).
func pruneOldestZips(dir string, keepLast int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}

	type fileInfo struct {
		name    string
		modTime time.Time
	}
	var zips []fileInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		zips = append(zips, fileInfo{name: e.Name(), modTime: fi.ModTime()})
	}
	if len(zips) <= keepLast {
		return nil
	}

	sort.Slice(zips, func(i, j int) bool { return zips[i].modTime.After(zips[j].modTime) })
	for _, f := range zips[keepLast:] {
		if err := os.Remove(filepath.Join(dir, f.name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", f.name, err)
		}
	}
	return nil
}
