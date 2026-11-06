//go:build windows

package evidence

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
)

func fileIdentity(f *os.File) (string, error) {
	var i windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &i); err != nil {
		return "", err
	}
	return fmt.Sprintf("%08x:%08x%08x", i.VolumeSerialNumber, i.FileIndexHigh, i.FileIndexLow), nil
}
