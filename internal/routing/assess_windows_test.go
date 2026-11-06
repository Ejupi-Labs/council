//go:build windows

package routing

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"golang.org/x/sys/windows"
)

func TestAssessmentRejectsNativeJunctionAncestor(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel.md"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Mkdir(alias, 0700); err != nil {
		t.Fatal(err)
	}
	p, err := windows.UTF16PtrFromString(alias)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	sub, _ := windows.UTF16FromString(`\??\` + outside)
	name, _ := windows.UTF16FromString(outside)
	data := make([]byte, 16+2*(len(sub)+len(name)))
	binary.LittleEndian.PutUint32(data[:4], windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(data[4:6], uint16(len(data)-8))
	binary.LittleEndian.PutUint16(data[10:12], uint16((len(sub)-1)*2))
	binary.LittleEndian.PutUint16(data[12:14], uint16(len(sub)*2))
	binary.LittleEndian.PutUint16(data[14:16], uint16((len(name)-1)*2))
	for i, c := range append(sub, name...) {
		binary.LittleEndian.PutUint16(data[16+i*2:], c)
	}
	var returned uint32
	err = windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT, &data[0], uint32(len(data)), nil, 0, &returned, nil)
	windows.CloseHandle(h)
	if err != nil {
		t.Fatal(err)
	}
	got, assessErr := Assess(context.Background(), root, `Replace "old" with "new" in alias/sentinel.md only`, domain.TaskVersion{}, "")
	if assessErr != nil || got.Classification != "unknown" || got.SourceProof != nil {
		t.Fatalf("junction target read %#v %v", got, assessErr)
	}
}
