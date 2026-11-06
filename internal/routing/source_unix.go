//go:build darwin || linux

package routing

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func sourceIdentity(f *os.File) (string, error) {
	var info unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &info); err != nil {
		return "", err
	}
	if info.Nlink != 1 {
		return "", errors.New("source must be a single-link ordinary file")
	}
	return fmt.Sprintf("%d:%d", info.Dev, info.Ino), nil
}
