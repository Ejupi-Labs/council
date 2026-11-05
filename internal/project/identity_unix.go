//go:build darwin || linux

package project

import (
	"fmt"
	"golang.org/x/sys/unix"
)

func platformPathValid(string) bool { return true }
func physicalIdentity(path string) (string, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return "", err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return "", fmt.Errorf("not a directory")
	}
	return fmt.Sprintf("unix:%x:%x", uint64(st.Dev), uint64(st.Ino)), nil
}
