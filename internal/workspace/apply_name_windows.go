//go:build windows

package workspace

import (
	"errors"
	"os"
	"strings"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Metadata-only access does not conflict with the exclusively held stage's data
// sharing policy, and never reads or follows a redirected target.
func applyMatchesName(parent *os.File, name string, file *os.File) (bool, error) {
	if parent == nil || file == nil || name == "" || strings.ContainsAny(name, "/\\:\x00") || !utf8.ValidString(name) {
		return false, ErrApplyUnsupported
	}
	value, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return false, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: windows.Handle(parent.Fd()), ObjectName: value, Attributes: windows.OBJ_DONT_REPARSE}
	var handle windows.Handle
	err = windows.NtCreateFile(&handle, windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE, &attributes, &windows.IO_STATUS_BLOCK{}, nil, windows.FILE_ATTRIBUTE_NORMAL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if status, ok := err.(windows.NTStatus); ok {
		err = status.Errno()
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(handle)
	var named, retained windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &named); err != nil {
		return false, err
	}
	if err = windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &retained); err != nil {
		return false, err
	}
	if named.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || named.NumberOfLinks != 1 || retained.NumberOfLinks != 1 {
		return false, ErrApplyUnsupported
	}
	return named.VolumeSerialNumber == retained.VolumeSerialNumber && named.FileIndexHigh == retained.FileIndexHigh && named.FileIndexLow == retained.FileIndexLow, nil
}
