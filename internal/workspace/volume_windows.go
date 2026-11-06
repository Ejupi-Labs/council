//go:build windows

package workspace

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
)

func validateSourceLayout(string) error { return nil }
func sameSourceVolume(r *os.Root, f *os.File) error {
	root, e := r.Open(".")
	if e != nil {
		return e
	}
	defer root.Close()
	var a, b windows.ByHandleFileInformation
	if e = windows.GetFileInformationByHandle(windows.Handle(root.Fd()), &a); e != nil {
		return e
	}
	if e = windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &b); e != nil {
		return e
	}
	if a.VolumeSerialNumber != b.VolumeSerialNumber {
		return fmt.Errorf("%w Source crosses an unsupported filesystem volume.", ErrBlocked)
	}
	return nil
}
