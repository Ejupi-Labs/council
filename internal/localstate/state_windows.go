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
	// A continuing execution owner retains os.OpenRoot, which does not share
	// DELETE access. MAXIMUM_ALLOWED requests that access, so do not reopen an
	// already-verified private directory for an unnecessary security mutation.
	// Any directory needing normalization still takes the no-propagation path
	// below and fails closed if its security handle cannot be acquired safely.
	if dir && verifyPrivate(path, true) == nil {
		return nil
	}
	f, err := openSecurityFile(path, dir)
	if err != nil {
		// A competing initializer can establish privacy and retain its root
		// after our initial read. Sharing failure is not privacy proof: only a
		// fresh complete read-only verification can make mutation unnecessary.
		if dir && errors.Is(err, windows.ERROR_SHARING_VIOLATION) && verifyPrivate(path, true) == nil {
			return nil
		}
		return err
	}
	defer f.Close()
	if err := protectSecurityHandle(f, dir); err != nil {
		return err
	}
	return checkPrivatePath(f, dir)
}
func verifyPrivate(path string, dir bool) error {
	f, err := openVerificationFile(path, dir)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := VerifyPrivateFile(f, dir); err != nil {
		return err
	}
	return checkPrivatePath(f, dir)
}

func openVerificationFile(path string, dir bool) (*os.File, error) {
	// Verification requests no mutation/DELETE rights and shares all access.
	// Otherwise an overlapping ordinary Go read handle would deny the DELETE
	// access in a first initializer's MAXIMUM_ALLOWED no-propagation handle.
	return openPrivateHandle(path, dir, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES)
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
