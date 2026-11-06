//go:build darwin || linux

package workspace

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func sameSourceVolume(r *os.Root, f *os.File) error {
	root, e := r.Open(".")
	if e != nil {
		return e
	}
	defer root.Close()
	var a, b unix.Stat_t
	if e = unix.Fstat(int(root.Fd()), &a); e != nil {
		return e
	}
	if e = unix.Fstat(int(f.Fd()), &b); e != nil {
		return e
	}
	if a.Dev != b.Dev {
		return fmt.Errorf("%w Source crosses an unsupported filesystem mount.", ErrBlocked)
	}
	return nil
}
