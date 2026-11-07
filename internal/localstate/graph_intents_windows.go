//go:build windows

package localstate

import (
	"golang.org/x/sys/windows"
	"os"
)

func graphIntentOpenRead(path string, dir bool) (*os.File, error) {
	access := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES)
	if !dir {
		access |= windows.GENERIC_READ
	}
	return openPrivateHandle(path, dir, access)
}
func graphIntentOpenLock(path string) (*os.File, error) {
	return openPrivateHandle(path, false, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES)
}
func graphIntentPublish(from, to string) error {
	a, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	b, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(a, b, windows.MOVEFILE_WRITE_THROUGH)
}
