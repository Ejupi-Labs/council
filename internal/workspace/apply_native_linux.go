//go:build linux

package workspace

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func applyUnixPlatformMetadata(file *os.File, st unix.Stat_t, directory bool) (string, error) {
	var extended unix.Statx_t
	if err := unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BASIC_STATS|unix.STATX_MNT_ID, &extended); err != nil {
		return "", fmt.Errorf("%w The mounted filesystem identity is unavailable: %v", ErrApplyUnsupported, err)
	}
	if extended.Mask&unix.STATX_MNT_ID == 0 || extended.Mnt_id == 0 {
		return "", fmt.Errorf("%w The mounted filesystem identity is unsupported.", ErrApplyUnsupported)
	}
	// A mount root is ordinary namespace information. Every other reported
	// inode attribute (including unknown future bits) is outside this copier.
	if extended.Attributes & ^uint64(unix.STATX_ATTR_MOUNT_ROOT) != 0 || extended.Attributes & ^extended.Attributes_mask != 0 {
		return "", fmt.Errorf("%w The file has unsupported inode attributes.", ErrApplyUnsupported)
	}
	if extended.Mask&unix.STATX_BASIC_STATS != unix.STATX_BASIC_STATS || extended.Ino != uint64(st.Ino) ||
		extended.Uid != st.Uid || extended.Gid != st.Gid || uint32(extended.Mode) != uint32(st.Mode) || uint64(extended.Nlink) != uint64(st.Nlink) {
		return "", fmt.Errorf("%w Native metadata changed or is incomplete.", ErrApplyConflict)
	}
	if size, err := unix.Flistxattr(int(file.Fd()), nil); err != nil || size != 0 {
		return "", fmt.Errorf("%w Extended attributes are present or unavailable: %v", ErrApplyUnsupported, err)
	}
	names := []string{"system.posix_acl_access"}
	if directory {
		names = append(names, "system.posix_acl_default")
	}
	for _, name := range names {
		if _, err := unix.Fgetxattr(int(file.Fd()), name, nil); !errors.Is(err, unix.ENODATA) {
			return "", fmt.Errorf("%w ACL metadata is present or unavailable: %v", ErrApplyUnsupported, err)
		}
	}
	return fmt.Sprintf("linux:%x:%x", uint64(st.Dev), extended.Mnt_id), nil
}

func applyUnixNewFileGroup(parent applyUnixMetadata) (uint32, error) {
	return applyLinuxNewFileGroup(parent.GID, uint32(os.Getegid()), parent.RawMode&unix.S_ISGID != 0)
}

func applyUnixRenameExclusive(sourceParent int, sourceName string, destinationParent int, destinationName string) error {
	return unix.Renameat2(sourceParent, sourceName, destinationParent, destinationName, unix.RENAME_NOREPLACE)
}
