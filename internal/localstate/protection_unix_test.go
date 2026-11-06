//go:build darwin || linux

package localstate

import (
	"fmt"
	"testing"

	"golang.org/x/sys/unix"
)

func protectionFingerprint(t *testing.T, path string) string {
	t.Helper()
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("uid=%d,gid=%d,mode=%o", st.Uid, st.Gid, st.Mode)
}
