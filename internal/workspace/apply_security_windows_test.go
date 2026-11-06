//go:build windows

package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"golang.org/x/sys/windows"
)

type nativeSecurityCase struct {
	source, stage, parent *os.File
	expected              *windows.SECURITY_DESCRIPTOR
	destination           string
}

func nativeSecurityOpen(t *testing.T, path string, access, disposition, share, flags uint32) *os.File {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, access, share, nil, disposition, flags|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	file := os.NewFile(uintptr(handle), path)
	t.Cleanup(func() { file.Close() })
	return file
}
func nativeSecurityQuery(t *testing.T, file *os.File) *windows.SECURITY_DESCRIPTOR {
	t.Helper()
	descriptor, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}
func nativeSecuritySet(t *testing.T, file *os.File, descriptor *windows.SECURITY_DESCRIPTOR, flags windows.SECURITY_INFORMATION) {
	t.Helper()
	owner, _, err := descriptor.Owner()
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		t.Fatal("Controlled DACL unavailable", err)
	}
	if err = windows.SetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|flags, owner, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
}
func inheritedSecurityCase(t *testing.T) nativeSecurityCase {
	t.Helper()
	base := t.TempDir()
	project := filepath.Join(base, "destination")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	parent := nativeSecurityOpen(t, project, windows.GENERIC_READ|windows.FILE_WRITE_DATA|windows.WRITE_DAC, windows.OPEN_EXISTING, share, windows.FILE_FLAG_BACKUP_SEMANTICS)
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	// A controlled parent differs from Council's private DACL. The file itself
	// inherits ordinary unprotected semantics; no real user directory is changed.
	descriptor, err := windows.SecurityDescriptorFromString("O:" + user.User.Sid.String() + "D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FR;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetSecurityInfo(windows.Handle(parent.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(project, "original.txt")
	if err = os.WriteFile(sourcePath, []byte("controlled text\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	source := nativeSecurityOpen(t, sourcePath, windows.GENERIC_READ|windows.READ_CONTROL, windows.OPEN_EXISTING, share, 0)
	expected := nativeSecurityQuery(t, source)
	control, _, err := expected.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED != 0 {
		t.Fatal("Controlled source lacks inherited semantics", err)
	}
	lock, err := localstate.Acquire(filepath.Join(base, "private"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lock.Close() })
	stage := nativeSecurityOpen(t, filepath.Join(lock.Root(), "staged.txt"), windows.GENERIC_READ|windows.GENERIC_WRITE|windows.DELETE|windows.READ_CONTROL|windows.WRITE_DAC|windows.WRITE_OWNER, windows.CREATE_NEW, 0, 0)
	if _, err = stage.Write([]byte("controlled text\r\n")); err != nil {
		t.Fatal(err)
	}
	if err = stage.Sync(); err != nil {
		t.Fatal(err)
	}
	return nativeSecurityCase{source: source, stage: stage, parent: parent, expected: expected, destination: filepath.Join(project, "published.txt")}
}

func TestApplyInheritedDACLDirectCopyCannotSubstitutePrivateParent(t *testing.T) {
	fixture := inheritedSecurityCase(t)
	nativeSecuritySet(t, fixture.stage, fixture.expected, 0)
	actual := nativeSecurityQuery(t, fixture.stage)
	if actual.String() == fixture.expected.String() {
		t.Fatal("Controlled negative fixture failed to distinguish private-parent inheritance")
	}
	if after := nativeSecurityQuery(t, fixture.source); after.String() != fixture.expected.String() {
		t.Fatal("Metadata preparation changed the user's original")
	}
	if _, err := os.Lstat(fixture.destination); !os.IsNotExist(err) {
		t.Fatal("Metadata-only refusal changed destination", err)
	}
}

func TestApplyInheritedDACLRestoresExactSemanticsWithinOneHandleRelativePublication(t *testing.T) {
	fixture := inheritedSecurityCase(t)
	temporary, cloneErr := windows.SecurityDescriptorFromString(fixture.expected.String())
	if cloneErr != nil {
		t.Fatal(cloneErr)
	}
	temporaryDACL, _, cloneErr := temporary.DACL()
	if cloneErr != nil || temporaryDACL == nil {
		t.Fatal("Temporary copied DACL unavailable", cloneErr)
	}
	for i := uint32(0); i < uint32(temporaryDACL.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if cloneErr = windows.GetAce(temporaryDACL, i, &ace); cloneErr != nil {
			t.Fatal(cloneErr)
		}
		ace.Header.AceFlags &^= windows.INHERITED_ACE
	}
	nativeSecuritySet(t, fixture.stage, temporary, windows.PROTECTED_DACL_SECURITY_INFORMATION)
	prepared := nativeSecurityQuery(t, fixture.stage)
	control, _, err := prepared.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("Temporary protected staging not established", err)
	}
	if expectedOwner, _, e := fixture.expected.Owner(); e != nil {
		t.Fatal(e)
	} else if actualOwner, _, e := prepared.Owner(); e != nil || !expectedOwner.Equals(actualOwner) {
		t.Fatal("Protected staging substituted owner", e)
	}
	expectedACEs := temporary.String()
	expectedACEs = expectedACEs[strings.Index(expectedACEs, "("):]
	actualACEs := prepared.String()
	actualACEs = actualACEs[strings.Index(actualACEs, "("):]
	if actualACEs != expectedACEs {
		t.Fatalf("Protected staging changed temporary exact ACEs: expected=%s; actual=%s; original=%s", expectedACEs, actualACEs, fixture.expected.String())
	}
	name, err := windows.UTF16FromString("published.txt")
	if err != nil {
		t.Fatal(err)
	}
	name = name[:len(name)-1]
	type renameInfo struct {
		Flags  uint32
		Root   windows.Handle
		Length uint32
		Name   [1]uint16
	}
	var layout renameInfo
	offset := unsafe.Offsetof(layout.Name)
	buffer := make([]byte, int(unsafe.Sizeof(layout))+2*(len(name)+1))
	*(*windows.Handle)(unsafe.Pointer(&buffer[unsafe.Offsetof(layout.Root)])) = windows.Handle(fixture.parent.Fd())
	*(*uint32)(unsafe.Pointer(&buffer[unsafe.Offsetof(layout.Length)])) = uint32(2 * len(name))
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[offset])), len(name)), name)
	// Flags/ReplaceIfExists remains zero: existing destinations cannot be overwritten.
	if err = windows.NtSetInformationFile(windows.Handle(fixture.stage.Fd()), &windows.IO_STATUS_BLOCK{}, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation); err != nil {
		t.Fatal("Handle-relative no-replace publication unavailable", err)
	}
	// This restore is part of the same already-permitted publish action. Stop
	// cannot skip it or authorize another publication; failures remain partial.
	nativeSecuritySet(t, fixture.stage, fixture.expected, windows.UNPROTECTED_DACL_SECURITY_INFORMATION)
	actual := nativeSecurityQuery(t, fixture.stage)
	expectedControl, _, err := fixture.expected.Control()
	if err != nil {
		t.Fatal(err)
	}
	actualControl, _, err := actual.Control()
	if err != nil {
		t.Fatal(err)
	}
	mask := windows.SECURITY_DESCRIPTOR_CONTROL(windows.SE_DACL_PRESENT | windows.SE_DACL_PROTECTED | windows.SE_DACL_AUTO_INHERITED | windows.SE_DACL_AUTO_INHERIT_REQ)
	if expectedControl&mask != actualControl&mask || fixture.expected.String() != actual.String() {
		t.Fatalf("Final metadata was not preserved: expected=%s control=%x; actual=%s control=%x", fixture.expected.String(), expectedControl, actual.String(), actualControl)
	}
	if err = fixture.stage.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(fixture.destination)
	if err != nil || string(data) != "controlled text\r\n" {
		t.Fatal("Published controlled bytes differ", err)
	}
}
