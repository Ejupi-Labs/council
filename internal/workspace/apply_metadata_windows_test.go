//go:build windows

package workspace

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func applyTestSetOptionalEA(t *testing.T, file *os.File) {
	t.Helper()
	name, value := []byte("COUNCIL005"), []byte("controlled optional metadata")
	// FILE_FULL_EA_INFORMATION: NextEntryOffset=0, Flags=0 (not FILE_NEED_EA),
	// one byte name length, two byte value length, name/NUL/value.
	buffer := make([]byte, 8+len(name)+1+len(value))
	buffer[5] = byte(len(name))
	binary.LittleEndian.PutUint16(buffer[6:8], uint16(len(value)))
	copy(buffer[8:], name)
	copy(buffer[8+len(name)+1:], value)
	var status windows.IO_STATUS_BLOCK
	if err := windows.NtSetEaFile(windows.Handle(file.Fd()), &status, &buffer[0], uint32(len(buffer))); err != nil {
		t.Fatal("creating the owned optional EA fixture failed", err)
	}
	actual := applyTestQueryEA(t, file)
	if len(actual) < len(buffer) || actual[4] != 0 || int(actual[5]) != len(name) || int(binary.LittleEndian.Uint16(actual[6:8])) != len(value) || !bytes.Equal(actual[8:8+len(name)], name) || !bytes.Equal(actual[8+len(name)+1:len(buffer)], value) {
		t.Fatalf("fixture did not create an actual optional Flags=0 EA: %x", actual)
	}
}

func applyTestQueryEA(t *testing.T, file *os.File) []byte {
	t.Helper()
	var status windows.IO_STATUS_BLOCK
	var buffer [4096]byte
	if err := windows.NtQueryEaFile(windows.Handle(file.Fd()), &status, &buffer[0], uint32(len(buffer)), false, nil, 0, nil, true); err != nil {
		t.Fatal("querying controlled EA fixture failed", err)
	}
	if status.Information < 8 || status.Information > uintptr(len(buffer)) {
		t.Fatal("controlled EA query returned an invalid length", status.Information)
	}
	return append([]byte{}, buffer[:status.Information]...)
}

func applyTestSetLowLabel(t *testing.T, file *os.File, inherited bool) {
	t.Helper()
	text := "S:(ML;;NW;;;LW)"
	if inherited {
		text = "S:(ML;OICI;NW;;;LW)"
	}
	descriptor, err := windows.SecurityDescriptorFromString(text)
	if err != nil {
		t.Fatal(err)
	}
	label, _, err := descriptor.SACL()
	if err != nil || label == nil {
		t.Fatal("controlled low-integrity label is unavailable", err)
	}
	if err = windows.SetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION, nil, nil, nil, label); err != nil {
		t.Fatal("creating owned low-integrity label failed", err)
	}
	actual, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	actualLabel, _, err := actual.SACL()
	if err != nil || actualLabel == nil || actualLabel.AceCount != 1 {
		t.Fatal("fixture did not create an actual low-integrity label", err)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err = windows.GetAce(actualLabel, 0, &ace); err != nil || ace == nil || ace.Header.AceType != 0x11 || ace.Mask != 1 || (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String() != "S-1-16-4096" {
		t.Fatal("fixture did not retain the low-integrity no-write-up SID/policy", err)
	}
	if inherited && ace.Header.AceFlags&(windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE) != windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE {
		t.Fatal("controlled parent label did not retain object/container inheritance")
	}
}

func applyTestSecurityWithLabel(t *testing.T, file *os.File) string {
	t.Helper()
	descriptor, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.LABEL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return descriptor.String()
}

func TestApplicationWindowsRejectsOptionalEAWithoutDestinationEffects(t *testing.T) {
	applyTestOriginalMetadataRefusal(t, applyTestSetOptionalEA, true)
}

func TestApplicationWindowsRejectsLowIntegrityLabelWithoutDestinationEffects(t *testing.T) {
	applyTestOriginalMetadataRefusal(t, func(t *testing.T, file *os.File) { applyTestSetLowLabel(t, file, false) }, false)
}

func applyTestOriginalMetadataRefusal(t *testing.T, set func(*testing.T, *os.File), hasEA bool) {
	t.Helper()
	m, repo, request := applicationFixture(t)
	name := filepath.Join(repo, "notes.txt")
	access := uint32(windows.GENERIC_READ | windows.FILE_WRITE_EA | windows.WRITE_OWNER)
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	file := nativeSecurityOpen(t, name, access, windows.OPEN_EXISTING, share, 0)
	set(t, file)
	before, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	beforeSecurity := applyTestSecurityWithLabel(t, file)
	var beforeEA []byte
	if hasEA {
		beforeEA = applyTestQueryEA(t, file)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	application, prepareErr := m.PrepareApply(context.Background(), request)
	if application != nil {
		if err = application.Close(); err != nil {
			t.Fatal("closing an unexecuted application failed", err)
		}
	}
	if !errors.Is(prepareErr, ErrApplyUnsupported) {
		t.Errorf("unsupported actual Windows metadata passed application preflight: %v", prepareErr)
	}
	actual, err := os.ReadFile(name)
	after, statErr := os.Stat(name)
	if err != nil || statErr != nil || string(actual) != "original text\r\n" || !os.SameFile(before, after) || before.Mode() != after.Mode() {
		t.Fatalf("metadata refusal changed original bytes/mode/identity: %q / %v / %v", actual, err, statErr)
	}
	observed := nativeSecurityOpen(t, name, windows.GENERIC_READ, windows.OPEN_EXISTING, share, 0)
	if actualSecurity := applyTestSecurityWithLabel(t, observed); actualSecurity != beforeSecurity {
		t.Fatal("metadata refusal changed original owner/DACL/integrity label")
	}
	if hasEA && !bytes.Equal(beforeEA, applyTestQueryEA(t, observed)) {
		t.Fatal("metadata refusal changed the actual optional EA")
	}
	if _, err = os.Lstat(filepath.Join(repo, "added.txt")); !os.IsNotExist(err) {
		t.Fatal("unsupported preflight published an addition", err)
	}
	if original, readErr := os.ReadFile(filepath.Join(repo, "delete.txt")); readErr != nil || string(original) != "retained deletion before bytes\r\n" {
		t.Fatal("unsupported preflight displaced a different original", readErr)
	}
}

func TestApplyWindowsLowIntegrityParentRefusesNewFileRule(t *testing.T) {
	path := t.TempDir()
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	parent := nativeSecurityOpen(t, path, windows.GENERIC_READ|windows.WRITE_OWNER, windows.OPEN_EXISTING, share, windows.FILE_FLAG_BACKUP_SEMANTICS)
	applyTestSetLowLabel(t, parent, true)
	before, err := parent.Stat()
	if err != nil {
		t.Fatal(err)
	}
	security := applyTestSecurityWithLabel(t, parent)
	if _, err = applyNewFileMetadata(parent, 0666); !errors.Is(err, ErrApplyUnsupported) {
		t.Errorf("new-file rule silently dropped actual parent integrity semantics: %v", err)
	}
	after, err := parent.Stat()
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || applyTestSecurityWithLabel(t, parent) != security {
		t.Fatal("unsupported parent metadata was normalized", err)
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		t.Fatal("new-file eligibility probe created destination effects", err)
	}
}
