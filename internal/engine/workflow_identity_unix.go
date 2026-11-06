//go:build darwin || linux

package engine

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func workflowFileIdentity(file *os.File) (string, error) {
	var info unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &info); err != nil {
		return "", err
	}
	return fmt.Sprintf("unix:%x:%x", uint64(info.Dev), uint64(info.Ino)), nil
}
