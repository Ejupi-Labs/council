//go:build windows

package cli

import (
	"encoding/binary"
	"os"

	"golang.org/x/sys/windows"
)

// Same real junction construction as localstate's owned ancestor-alias fixture;
// this requires no optional symbolic-link privilege or external shell.
func graphIntentTestAlias(target, alias string) error {
	if err := os.Mkdir(alias, 0700); err != nil {
		return err
	}
	path, err := windows.UTF16PtrFromString(alias)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(path, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	substitute, err := windows.UTF16FromString(`\??\` + target)
	if err != nil {
		return err
	}
	printName, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	data := make([]byte, 16+2*(len(substitute)+len(printName)))
	binary.LittleEndian.PutUint32(data[0:4], windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(data[4:6], uint16(len(data)-8))
	binary.LittleEndian.PutUint16(data[10:12], uint16((len(substitute)-1)*2))
	binary.LittleEndian.PutUint16(data[12:14], uint16(len(substitute)*2))
	binary.LittleEndian.PutUint16(data[14:16], uint16((len(printName)-1)*2))
	for i, c := range append(substitute, printName...) {
		binary.LittleEndian.PutUint16(data[16+i*2:], c)
	}
	var returned uint32
	return windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT, &data[0], uint32(len(data)), nil, 0, &returned, nil)
}

func graphIntentTestMakePublic(path string) error {
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
