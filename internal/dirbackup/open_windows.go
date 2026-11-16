//go:build windows

package dirbackup

import (
	"errors"
	"io/fs"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// openSharedRead opens a file using Windows FILE_SHARE_READ|WRITE|DELETE so files
// currently open for reading/appending by other applications can still be backed up.
func openSharedRead(path string) (*os.File, error) {
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	shareMode := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	flags := uint32(windows.FILE_ATTRIBUTE_NORMAL | windows.FILE_FLAG_SEQUENTIAL_SCAN)
	h, err := windows.CreateFile(p16, windows.GENERIC_READ, shareMode, nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}

func isReparsePoint(path string, d fs.DirEntry) bool {
	if d.Type()&os.ModeSymlink != 0 {
		return true
	}
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := windows.GetFileAttributes(p16)
	if err != nil {
		return false
	}
	return attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func isLockOrPermissionError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, os.ErrPermission) ||
		errors.Is(err, syscall.EACCES)
}
