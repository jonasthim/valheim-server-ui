//go:build windows

package api

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// diskUsage returns the free/total bytes of the volume containing path.
func diskUsage(path string) (free, total int64, err error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("disk usage %s: %w", path, err)
	}
	var availToCaller, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &availToCaller, &totalBytes, &totalFree); err != nil {
		return 0, 0, fmt.Errorf("disk usage %s: %w", path, err)
	}
	return int64(availToCaller), int64(totalBytes), nil //nolint:gosec // volume sizes fit in int64
}
