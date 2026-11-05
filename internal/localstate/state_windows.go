//go:build windows

package localstate

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"unsafe"
)

func currentSID() (*windows.SID, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return u.User.Sid, nil
}
func securePrivate(path string, dir bool) error {
	sid, err := currentSID()
	if err != nil {
		return err
	}
	existing, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := existing.Owner()
	if err != nil || owner == nil || !owner.Equals(sid) {
		return errors.New("Private state belongs to another Windows user.")
	}
	flags := ""
	if dir {
		flags = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;" + flags + ";FA;;;" + sid.String() + ")(A;" + flags + ";FA;;;SY)(A;" + flags + ";FA;;;BA)")
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return err
	}
	return verifyPrivate(path, dir)
}
func verifyPrivate(path string, dir bool) error {
	i, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if i.Mode()&os.ModeSymlink != 0 || i.IsDir() != dir || (!dir && !i.Mode().IsRegular()) {
		return errors.New("Private state must be an ordinary owned file or directory.")
	}
	sid, err := currentSID()
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !owner.Equals(sid) {
		return errors.New("Private state belongs to another Windows user.")
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("Private state DACL is not protected.")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.New("Private state DACL is absent.")
	}
	system, _ := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	admins, _ := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	seenOwner := false
	// GetAce reports end-of-ACL with ERROR_INVALID_PARAMETER. The ACL's ACE count is
	// intentionally not exposed by x/sys; enumerate with a strict finite bound.
	for n := uint32(0); n < 1024; n++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		err := windows.GetAce(acl, n, &ace)
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			if seenOwner {
				return nil
			}
			break
		}
		if err != nil {
			return err
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("Unexpected private state access rule.")
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if aceSID.Equals(sid) {
			seenOwner = true
		} else if !aceSID.Equals(system) && !aceSID.Equals(admins) {
			return errors.New("Private state grants access to another Windows principal.")
		}
	}
	return errors.New("Private state owner access rule is missing.")
}
func lockNative(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}
func unlockNative(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &windows.Overlapped{})
}
func isLockBusy(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING)
}
func atomicReplace(from, to string) error {
	a, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	b, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func syncDirectory(string) error { return nil } // MoveFileEx WRITE_THROUGH flushes publication on Windows.
