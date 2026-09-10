//go:build windows

package logs

import "os"

// fileIdentity distinguishes the file backing an open path across rotations.
// os.FileInfo carries no file index on Windows, so every file has the same
// identity there and rotation is detected by the size-shrink check alone
// (console.log is rotated by rename before the next start, so the file the
// tailer holds is replaced by an empty one, which that check catches).
type fileIdentity struct{}

func identify(os.FileInfo) fileIdentity { return fileIdentity{} }
