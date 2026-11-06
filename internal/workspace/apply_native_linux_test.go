//go:build linux

package workspace

import (
	"encoding/binary"
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestApplyLinuxRefusesAccessAndDefaultACL(t *testing.T) {
	_, directory := applyTestDirectory(t)
	file, err := applyOpenRelative(directory, "acl-file", true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = applyNativeInfo(file, false); err != nil {
		t.Fatal(err)
	}
	// A named user and mask make this an actual extended ACL; a mode-only
	// three-entry ACL can be normalized away by the kernel.
	acl := make([]byte, 4+5*8)
	binary.LittleEndian.PutUint32(acl[:4], 2)
	for index, entry := range []struct {
		tag, permissions uint16
		id               uint32
	}{
		{1, 6, ^uint32(0)}, {2, 4, uint32(os.Geteuid()) + 1}, {4, 4, ^uint32(0)}, {16, 4, ^uint32(0)}, {32, 0, ^uint32(0)},
	} {
		start := 4 + index*8
		binary.LittleEndian.PutUint16(acl[start:start+2], entry.tag)
		binary.LittleEndian.PutUint16(acl[start+2:start+4], entry.permissions)
		binary.LittleEndian.PutUint32(acl[start+4:start+8], entry.id)
	}
	if err = unix.Fsetxattr(int(file.Fd()), "system.posix_acl_access", acl, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = applyNativeInfo(file, false); !errors.Is(err, ErrApplyUnsupported) {
		t.Fatalf("extended access ACL was accepted: %v", err)
	}
	if err = unix.Fsetxattr(int(directory.Fd()), "system.posix_acl_default", acl, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = applyNewFileMetadata(directory, 0644); !errors.Is(err, ErrApplyUnsupported) {
		t.Fatalf("new-file destination default ACL was accepted: %v", err)
	}
}
