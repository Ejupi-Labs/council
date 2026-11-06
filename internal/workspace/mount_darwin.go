//go:build darwin

package workspace

import (
	"fmt"
	"golang.org/x/sys/unix"
	"path/filepath"
)

func validateSourceLayout(repo string) error {
	n, e := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if e != nil || n > 100000 {
		return fmt.Errorf("%w Source mount layout cannot be verified.", ErrBlocked)
	}
	buf := make([]unix.Statfs_t, n+16)
	n, e = unix.Getfsstat(buf, unix.MNT_NOWAIT)
	if e != nil || n > len(buf) {
		return fmt.Errorf("%w Source mount layout changed during observation.", ErrBlocked)
	}
	for _, st := range buf[:n] {
		mount := filepath.Clean(unix.ByteSliceToString(st.Mntonname[:]))
		if mount != repo && within(repo, mount) {
			return fmt.Errorf("%w Nested source mounts cannot be captured safely.", ErrBlocked)
		}
	}
	return nil
}
