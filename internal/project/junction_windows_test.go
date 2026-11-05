//go:build windows

package project

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// A directory junction exercises a real Windows ancestor alias without depending on
// the host's optional symbolic-link evaluation privilege or policy.
func createJunctionAlias(target, alias string) error {
	if err := os.Mkdir(alias, 0700); err != nil {
		return err
	}
	path, err := windows.UTF16PtrFromString(alias)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(path, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	substitute, err := windows.UTF16FromString(`\??\` + target)
	if err != nil {
		return err
	}
	printName, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	data := make([]byte, 16+2*(len(substitute)+len(printName)))
	binary.LittleEndian.PutUint32(data[0:4], windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(data[4:6], uint16(len(data)-8))
	binary.LittleEndian.PutUint16(data[10:12], uint16((len(substitute)-1)*2))
	binary.LittleEndian.PutUint16(data[12:14], uint16(len(substitute)*2))
	binary.LittleEndian.PutUint16(data[14:16], uint16((len(printName)-1)*2))
	for i, c := range append(substitute, printName...) {
		binary.LittleEndian.PutUint16(data[16+i*2:], c)
	}
	var returned uint32
	return windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT, &data[0], uint32(len(data)), nil, 0, &returned, nil)
}

func TestInspectWindowsJunctionAlias(t *testing.T) {
	root := repository(t)
	sub := filepath.Join(root, "selected folder")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	direct, err := Inspect(context.Background(), sub, root)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "junction")
	if err := createJunctionAlias(root, alias); err != nil {
		t.Fatal(err)
	}
	observed, err := Inspect(context.Background(), filepath.Join(alias, "selected folder"), filepath.Dir(alias))
	if err != nil {
		t.Fatalf("accessible junction project rejected: %v", err)
	}
	if observed.Identity != direct.Identity || observed.Observation.ResolvedPath != direct.Observation.ResolvedPath || observed.Observation.RepositoryRoot != direct.Observation.RepositoryRoot {
		t.Fatalf("junction project changed canonical path or physical identity: got=%+v direct=%+v", observed, direct)
	}
}
