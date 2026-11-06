//go:build darwin

package workspace

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// This is the same cgo-free libSystem bridge used by x/sys/unix's Darwin
// wrappers. Direct Darwin kernel syscall traps are not a supported fallback.
//
//go:linkname applyDarwinLibcSyscall6 syscall.syscall6
//go:uintptrescapes
func applyDarwinLibcSyscall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err syscall.Errno)

//go:cgo_import_dynamic council_apply_fgetattrlist fgetattrlist "/usr/lib/libSystem.B.dylib"

var applyDarwinFgetattrlistAddr uintptr

func applyUnixPlatformMetadata(file *os.File, st unix.Stat_t, _ bool) (string, error) {
	if st.Flags != 0 {
		return "", fmt.Errorf("%w Darwin file flags cannot be preserved.", ErrApplyUnsupported)
	}
	var filesystem unix.Statfs_t
	if err := unix.Fstatfs(int(file.Fd()), &filesystem); err != nil {
		return "", err
	}
	mountEnd := bytes.IndexByte(filesystem.Mntonname[:], 0)
	if mountEnd < 1 || filesystem.Mntonname[0] != '/' || filesystem.Fsid.Val == [2]int32{} {
		return "", fmt.Errorf("%w The Darwin mount identity is invalid.", ErrApplyUnsupported)
	}
	if size, err := unix.Flistxattr(int(file.Fd()), nil); err != nil || size != 0 {
		return "", fmt.Errorf("%w Extended attributes are present or unavailable: %v", ErrApplyUnsupported, err)
	}
	attributes := unix.Attrlist{Bitmapcount: 5, Commonattr: unix.ATTR_CMN_EXTENDED_SECURITY}
	var buffer [8192]byte
	_, _, errno := applyDarwinLibcSyscall6(applyDarwinFgetattrlistAddr, file.Fd(), uintptr(unsafe.Pointer(&attributes)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), unix.FSOPT_REPORT_FULLSIZE, 0)
	runtime.KeepAlive(file)
	runtime.KeepAlive(&attributes)
	runtime.KeepAlive(&buffer)
	if errno != 0 {
		return "", fmt.Errorf("%w Darwin ACL metadata is unavailable: %v", ErrApplyUnsupported, errno)
	}
	if err := applyParseDarwinACL(buffer[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("darwin:%x:%x:%x:%s", uint32(st.Dev), uint32(filesystem.Fsid.Val[0]), uint32(filesystem.Fsid.Val[1]), filesystem.Mntonname[:mountEnd]), nil
}

func applyUnixNewFileGroup(parent applyUnixMetadata) (uint32, error) {
	// Darwin's creation rule inherits the enclosing directory's group.
	return parent.GID, nil
}

func applyUnixRenameExclusive(sourceParent int, sourceName string, destinationParent int, destinationName string) error {
	return unix.RenameatxNp(sourceParent, sourceName, destinationParent, destinationName, unix.RENAME_EXCL)
}
