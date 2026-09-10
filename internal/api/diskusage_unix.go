//go:build !windows

package api

import (
	"fmt"
	"syscall"
)

// diskUsage returns the free/total bytes of the filesystem containing path.
func diskUsage(path string) (free, total int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	//nolint:gosec,unconvert // Bsize/Bavail/Blocks differ in type and signedness across unix platforms; disk sizes fit in int64.
	free = int64(stat.Bavail) * int64(stat.Bsize)
	//nolint:gosec,unconvert // see above
	total = int64(stat.Blocks) * int64(stat.Bsize)
	return free, total, nil
}
