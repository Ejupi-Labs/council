//go:build windows

package workspace

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"golang.org/x/sys/windows"
)

// These are actual retained-handle replacement fixtures, including an ambient
// temporary parent rather than only the normalized parent used by the original
// native prerequisite. All security changes are confined to this test's files.
func TestApplyWindowsReplacementRestoresParentInheritanceVariants(t *testing.T) {
	for _, variant := range []string{"ambient", "creator_owner", "explicit_and_inherited", "legacy_control"} {
		t.Run(variant, func(t *testing.T) {
			base := t.TempDir()
			projectPath := filepath.Join(base, "destination")
			if err := os.Mkdir(projectPath, 0700); err != nil {
				t.Fatal(err)
			}
			share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
			parent := nativeSecurityOpen(t, projectPath, windows.GENERIC_READ|windows.FILE_WRITE_DATA|windows.WRITE_DAC, windows.OPEN_EXISTING, share, windows.FILE_FLAG_BACKUP_SEMANTICS)
			if variant == "creator_owner" || variant == "explicit_and_inherited" {
				user, err := windows.GetCurrentProcessToken().GetTokenUser()
				if err != nil {
					t.Fatal(err)
				}
				parentSD, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICIIO;FA;;;CO)")
				if err != nil {
					t.Fatal(err)
				}
				acl, _, err := parentSD.DACL()
				if err != nil {
					t.Fatal(err)
				}
				if err = windows.SetSecurityInfo(windows.Handle(parent.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
					t.Fatal(err)
				}
			}
			sourcePath := filepath.Join(projectPath, "original.txt")
			if err := os.WriteFile(sourcePath, []byte("original controlled text\r\n"), 0644); err != nil {
				t.Fatal(err)
			}
			source := nativeSecurityOpen(t, sourcePath, windows.GENERIC_READ|windows.DELETE|windows.READ_CONTROL|windows.WRITE_DAC, windows.OPEN_EXISTING, share, 0)
			if variant == "explicit_and_inherited" {
				explicit, err := windows.SecurityDescriptorFromString("D:(A;;FR;;;WD)")
				if err != nil {
					t.Fatal(err)
				}
				acl, _, err := explicit.DACL()
				if err != nil {
					t.Fatal(err)
				}
				if err = windows.SetSecurityInfo(windows.Handle(source.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
					t.Fatal(err)
				}
			}
			if variant == "legacy_control" {
				applyWindowsLegacyDescriptorFixture(t, source)
			}
			expected, err := applyNativeInfo(source, false)
			if err != nil {
				t.Fatal(err)
			}
			parentBefore, err := applyNativeInfo(parent, true)
			if err != nil {
				t.Fatal(err)
			}
			lock, err := localstate.Acquire(filepath.Join(base, "private"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { lock.Close() })
			private, err := applyOpenDirectory(lock.Root())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { private.Close() })
			stage, err := applyOpenRelative(private, "staged.txt", true, false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { stage.Close() })
			if _, err = stage.Write([]byte("replacement controlled text\r\n")); err != nil {
				t.Fatal(err)
			}
			if err = applyPrepareMetadata(stage, expected); err != nil {
				t.Fatal("protected metadata preparation", err)
			}
			if err = applyRenameExclusive(parent, "original.txt", source, private, "quarantine.txt"); err != nil {
				t.Fatal("quarantine original", err)
			}
			if err = applyRenameExclusive(private, "staged.txt", stage, parent, "original.txt"); err != nil {
				t.Fatal("publish replacement", err)
			}
			if err = applyRestoreMetadata(stage, expected); err != nil {
				t.Fatal("exact original metadata restoration", err)
			}
			actual, err := applyNativeInfo(stage, false)
			if err != nil || !applyMetadataEqual(actual, expected) {
				t.Fatal("published replacement changed exact original metadata", err)
			}
			originalAfter, err := applyNativeInfo(source, false)
			if err != nil || !applyMetadataEqual(originalAfter, expected) || originalAfter.Identity != expected.Identity {
				t.Fatal("quarantine changed retained original metadata or identity", err)
			}
			parentAfter, err := applyNativeInfo(parent, true)
			if err != nil || !applyMetadataEqual(parentAfter, parentBefore) || parentAfter.Identity != parentBefore.Identity {
				t.Fatal("publication changed the parent metadata or identity", err)
			}
			if err = stage.Close(); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(sourcePath); err != nil || string(data) != "replacement controlled text\r\n" {
				t.Fatal("replacement bytes differ", err)
			}
			if data, err := applyReadStable(source); err != nil || string(data) != "original controlled text\r\n" {
				t.Fatal("retained quarantine bytes differ", err)
			}
		})
	}
}

func applyWindowsLegacyDescriptorFixture(t *testing.T, source *os.File) {
	t.Helper()
	// The obsolete path setter is restricted to constructing this owned fixture;
	// it is never the production application setter.
	sd := nativeSecurityQuery(t, source)
	mask := windows.SECURITY_DESCRIPTOR_CONTROL(windows.SE_DACL_AUTO_INHERITED | windows.SE_DACL_AUTO_INHERIT_REQ)
	if err := sd.SetControl(mask, 0); err != nil {
		t.Fatal(err)
	}
	path, err := windows.UTF16PtrFromString(source.Name())
	if err != nil {
		t.Fatal(err)
	}
	setFileSecurity := windows.NewLazySystemDLL("advapi32.dll").NewProc("SetFileSecurityW")
	ok, _, callErr := setFileSecurity.Call(uintptr(unsafe.Pointer(path)), uintptr(windows.DACL_SECURITY_INFORMATION), uintptr(unsafe.Pointer(sd)))
	runtime.KeepAlive(sd)
	if ok == 0 {
		t.Fatal("legacy descriptor fixture could not be established", callErr)
	}
	control, _, err := nativeSecurityQuery(t, source).Control()
	if err != nil || control&(windows.SE_DACL_AUTO_INHERITED|windows.SE_DACL_PROTECTED) != 0 {
		t.Fatalf("legacy descriptor fixture retained current-model control: %04x / %v", control, err)
	}
}

// The direct native setter is a prerequisite only for legacy descriptors.
// The observed current-model negative is retained in the qualification log;
// OWNER|DACL alone must not be substituted for both descriptor models.
func TestApplyWindowsNativeSecuritySetterPreservesLegacyDescriptorModel(t *testing.T) {
	testApplyWindowsSecurityRestorationModel(t, true, true, false)
}

func TestApplyWindowsSelectedSecuritySetterPreservesModelAndFutureInheritance(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "current_model"
		if legacy {
			name = "legacy_model"
		}
		t.Run(name, func(t *testing.T) {
			testApplyWindowsSecurityRestorationModel(t, legacy, false, true)
		})
	}
}

func testApplyWindowsSecurityRestorationModel(t *testing.T, legacy, nativeOnly, futureInheritance bool) {
	t.Helper()
	fixture := inheritedSecurityCase(t)
	if legacy {
		applyWindowsLegacyDescriptorFixture(t, fixture.source)
	}
	expected, err := applyNativeInfo(fixture.source, false)
	if err != nil {
		t.Fatal(err)
	}
	if !legacy && expected.Control&uint32(windows.SE_DACL_AUTO_INHERITED) == 0 {
		t.Fatal("current-model fixture did not establish auto-inheritance")
	}
	parentBefore, err := applyNativeInfo(fixture.parent, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = applyPrepareMetadata(fixture.stage, expected); err != nil {
		t.Fatal("protected stage preparation", err)
	}
	if err = applyRenameExclusive(nil, "staged.txt", fixture.stage, fixture.parent, "published.txt"); err != nil {
		t.Fatal("native exclusive publication", err)
	}
	descriptor := expected.Native.(*windows.SECURITY_DESCRIPTOR)
	if nativeOnly {
		setter := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtSetSecurityObject")
		status, _, _ := setter.Call(fixture.stage.Fd(), uintptr(windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION), uintptr(unsafe.Pointer(descriptor)))
		runtime.KeepAlive(descriptor)
		if status != 0 {
			t.Fatal("native handle security setter", windows.NTStatus(status).Errno())
		}
	} else if err = applyRestoreMetadata(fixture.stage, expected); err != nil {
		t.Fatal("production descriptor-model restoration", err)
	}
	actual, err := applyNativeInfo(fixture.stage, false)
	if err != nil || !applyMetadataEqual(actual, expected) {
		t.Fatalf("selected setter changed exact descriptor model: expected_control=%04x actual_control=%04x security_equal=%t owner_equal=%t / %v", expected.Control, actual.Control, actual.Security == expected.Security, actual.Owner == expected.Owner, err)
	}
	parentAfter, err := applyNativeInfo(fixture.parent, true)
	if err != nil || !applyMetadataEqual(parentAfter, parentBefore) || parentAfter.Identity != parentBefore.Identity {
		t.Fatal("selected setter changed original parent", err)
	}
	originalAfter, err := applyNativeInfo(fixture.source, false)
	if err != nil || !applyMetadataEqual(originalAfter, expected) || originalAfter.Identity != expected.Identity {
		t.Fatal("selected setter changed retained original", err)
	}
	if err = fixture.stage.Close(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(fixture.destination); err != nil || string(data) != "controlled text\r\n" {
		t.Fatal("selected setter changed published bytes", err)
	}
	if !futureInheritance {
		return
	}
	// The original remains an untouched sibling with the captured descriptor.
	// Closing the exclusive stage above permits the OS to propagate this one
	// controlled parent change to BOTH ordinary files; no target normalization
	// or privilege change is involved.
	parentSD := nativeSecurityQuery(t, fixture.parent)
	updated, err := windows.SecurityDescriptorFromString(parentSD.String() + "(A;OICI;FR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := updated.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetSecurityInfo(windows.Handle(fixture.parent.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal("controlled future parent policy update", err)
	}
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	published := nativeSecurityOpen(t, fixture.destination, windows.GENERIC_READ|windows.READ_CONTROL, windows.OPEN_EXISTING, share, 0)
	changed, err := applyNativeInfo(published, false)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := applyNativeInfo(fixture.source, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Security == expected.Security || reference.Security == expected.Security {
		t.Fatal("controlled parent update did not actually affect both siblings")
	}
	if !applyMetadataEqual(changed, reference) || changed.Control&uint32(windows.SE_DACL_PROTECTED) != 0 || reference.Identity != expected.Identity || changed.Identity != actual.Identity {
		t.Fatalf("replacement changed future inheritance semantics: security_equal=%t control=%04x/%04x identities_equal=%t/%t", changed.Security == reference.Security, changed.Control, reference.Control, changed.Identity == actual.Identity, reference.Identity == expected.Identity)
	}
	if data, err := applyReadStable(fixture.source); err != nil || string(data) != "controlled text\r\n" {
		t.Fatal("parent update changed original sibling bytes", err)
	}
	if data, err := applyReadStable(published); err != nil || string(data) != "controlled text\r\n" {
		t.Fatal("parent update changed published bytes", err)
	}
}
