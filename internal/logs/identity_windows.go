//go:build windows

package logs

import (
	"os"

	"golang.org/x/sys/windows"
)

// fileIdentity distinguishes the file backing an open path across rotations:
// volume serial number plus the NTFS file index, the Windows equivalent of
// dev+inode.
type fileIdentity struct {
	volume uint32
	index  uint64
}

// identify opens path just long enough to read its file index. os.FileInfo
// does not carry it on Windows.
func identify(path string, _ os.FileInfo) fileIdentity {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fileIdentity{}
	}
	h, err := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return fileIdentity{}
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return fileIdentity{}
	}
	return fileIdentity{volume: info.VolumeSerialNumber, index: uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)}
}

// openForTail opens path for reading with FILE_SHARE_DELETE, so the manager
// can still rotate console.log (rename it away) while it is being tailed;
// os.Open would make every rename fail with a sharing violation.
func openForTail(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
