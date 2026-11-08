//go:build darwin

package historyout

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func nativeVolume(f *os.File) (string, error) {
	var st unix.Statfs_t
	if err := unix.Fstatfs(int(f.Fd()), &st); err != nil {
		return "", err
	}
	if st.Fsid.Val == [2]int32{} {
		return "", ErrUnsupported
	}
	return fmt.Sprintf("%x:%x", uint32(st.Fsid.Val[0]), uint32(st.Fsid.Val[1])), nil
}
