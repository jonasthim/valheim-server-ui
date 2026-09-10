//go:build !windows

package logs

import (
	"os"
	"syscall"
)

// fileIdentity distinguishes the file backing an open path across rotations.
type fileIdentity struct {
	dev, ino uint64
}

func identify(fi os.FileInfo) fileIdentity {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return fileIdentity{dev: uint64(st.Dev), ino: uint64(st.Ino)} //nolint:unconvert // Dev is int32 on some platforms
	}
	return fileIdentity{}
}
