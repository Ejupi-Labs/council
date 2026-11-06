//go:build darwin || linux

package evidence

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func fileIdentity(f *os.File) (string, error) {
	var i unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &i); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d", i.Dev, i.Ino), nil
}
