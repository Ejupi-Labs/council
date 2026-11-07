//go:build darwin || linux

package workspace

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func graphContextFileIdentity(file *os.File) (string, error) {
	var info unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &info); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d", info.Dev, info.Ino), nil
}

func graphContextOpenRead(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
}
