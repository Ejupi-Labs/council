//go:build linux

package historyout

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func nativeVolume(f *os.File) (string, error) {
	var st unix.Statx_t
	if err := unix.Statx(int(f.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BASIC_STATS|unix.STATX_MNT_ID, &st); err != nil {
		return "", fmt.Errorf("%w: mount identity unavailable", ErrUnsupported)
	}
	if st.Mask&unix.STATX_MNT_ID == 0 || st.Mnt_id == 0 {
		return "", ErrUnsupported
	}
	return fmt.Sprintf("%x:%x:%x", st.Dev_major, st.Dev_minor, st.Mnt_id), nil
}
