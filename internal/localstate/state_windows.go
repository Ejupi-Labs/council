//go:build windows

package localstate

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"runtime"
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
	if err != nil || owner == nil {
		return errors.New("Private state belongs to another Windows user.")
	}
	securityInfo := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	var normalizedOwner *windows.SID
	if !owner.Equals(sid) {
		token := windows.GetCurrentProcessToken()
		defaultOwner, err := tokenDefaultOwner(token)
		if err != nil {
			return err
		}
		groups, err := token.GetTokenGroups()
		if err != nil {
			return err
		}
		if !admissibleOwner(owner, sid, defaultOwner, groups.AllGroups()) {
			return errors.New("Private state belongs to another Windows user.")
		}
		// Elevated tokens can create objects owned by their default owner group.
		// Normalize only that exact eligible owner before database or token writes.
		securityInfo |= windows.OWNER_SECURITY_INFORMATION
		normalizedOwner = sid
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
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, securityInfo, normalizedOwner, nil, acl, nil); err != nil {
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

func tokenDefaultOwner(token windows.Token) (*windows.SID, error) {
	var size uint32
	err := windows.GetTokenInformation(token, windows.TokenOwner, nil, 0, &size)
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || size < uint32(unsafe.Sizeof(uintptr(0))) || size > 64*1024 {
		return nil, errors.New("Cannot establish the Windows token's default owner.")
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(token, windows.TokenOwner, &buffer[0], uint32(len(buffer)), &size); err != nil {
		return nil, err
	}
	owner := *(**windows.SID)(unsafe.Pointer(&buffer[0]))
	if owner == nil || !owner.IsValid() {
		return nil, errors.New("The Windows token default owner is invalid.")
	}
	copied, err := owner.Copy()
	runtime.KeepAlive(buffer)
	return copied, err
}

// Default owner admission is an exact token fact, never an Administrators membership fallback.
func admissibleOwner(owner, user, defaultOwner *windows.SID, groups []windows.SIDAndAttributes) bool {
	if owner == nil || user == nil {
		return false
	}
	if owner.Equals(user) {
		return true
	}
	if defaultOwner == nil || defaultOwner.IsWellKnown(windows.WinLocalSystemSid) || !owner.Equals(defaultOwner) {
		return false
	}
	for _, group := range groups {
		if group.Sid != nil && group.Sid.Equals(defaultOwner) &&
			group.Attributes&windows.SE_GROUP_OWNER != 0 &&
			group.Attributes&windows.SE_GROUP_USE_FOR_DENY_ONLY == 0 {
			return true
		}
	}
	return false
}
