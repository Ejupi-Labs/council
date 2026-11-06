//go:build windows

package localstate

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"unsafe"
)

// Go's ordinary file handles do not request WRITE_DAC. Reopen for security
// access, require the retained handles to identify the same single-link object,
// and apply owner/DACL changes to that handle rather than a replaceable path.
func ProtectPrivateFile(f *os.File, dir bool) error {
	if err := checkPrivatePath(f, dir); err != nil {
		return err
	}
	securityFile, err := openSecurityFile(f.Name(), dir)
	if err != nil {
		return err
	}
	defer securityFile.Close()
	original, err := f.Stat()
	if err != nil {
		return err
	}
	securityInfo, err := securityFile.Stat()
	if err != nil || !os.SameFile(original, securityInfo) {
		return errors.New("Private state changed before protection.")
	}
	if err := protectSecurityHandle(securityFile, dir); err != nil {
		return err
	}
	if err := checkPrivatePath(f, dir); err != nil {
		return err
	}
	return VerifyPrivateFile(f, dir)
}

func openSecurityFile(path string, dir bool) (*os.File, error) {
	access := uint32(windows.READ_CONTROL | windows.WRITE_DAC | windows.WRITE_OWNER | windows.FILE_READ_ATTRIBUTES)
	if dir {
		// SetSecurityInfo deliberately suppresses propagation to existing children
		// for MAXIMUM_ALLOWED handles. Their identities/link counts are not yet
		// checked; updating inherited ACEs could mutate an outside hard-link target.
		// New children still inherit this directory's restricted OICI entries.
		access = windows.MAXIMUM_ALLOWED
	}
	return openPrivateHandle(path, dir, access)
}

// openPrivateHandle checks the object behind the retained, non-inheritable
// handle. Both readers and normalizers share all access; desired rights differ.
func openPrivateHandle(path string, dir bool, access uint32) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 || before.IsDir() != dir || (!dir && !before.Mode().IsRegular()) {
		return nil, errors.New("Private state must be an ordinary owned file or directory.")
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		f.Close()
		return nil, errors.New("Private state changed before protection.")
	}
	if err := checkPrivatePath(f, dir); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func verifySingleLink(f *os.File) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return err
	}
	if info.NumberOfLinks != 1 {
		return errors.New("Private state files must have exactly one hard link.")
	}
	return nil
}

func protectSecurityHandle(f *os.File, dir bool) error {
	if err := checkPrivateObject(f, dir); err != nil {
		return err
	}
	sid, err := currentSID()
	if err != nil {
		return err
	}
	existing, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
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
		// Only the exact eligible token default owner may be normalized.
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
	if err := windows.SetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT, securityInfo, normalizedOwner, nil, acl, nil); err != nil {
		return err
	}
	return VerifyPrivateFile(f, dir)
}

func VerifyPrivateFile(f *os.File, dir bool) error {
	if err := checkPrivateObject(f, dir); err != nil {
		return err
	}
	i, err := f.Stat()
	if err != nil {
		return err
	}
	if i.IsDir() != dir || !dir && !i.Mode().IsRegular() {
		return errors.New("Private state must be an ordinary owned file or directory.")
	}
	sid, err := currentSID()
	if err != nil {
		return err
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
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
		if dir {
			// SQLite creates its own companions without a supplied descriptor.
			// Their restricted access must inherit to files and nested directories;
			// a private but non-inheritable root would fall back to token defaults.
			required := byte(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE)
			forbidden := byte(windows.INHERIT_ONLY_ACE | windows.NO_PROPAGATE_INHERIT_ACE)
			if ace.Header.AceFlags&required != required || ace.Header.AceFlags&forbidden != 0 {
				return errors.New("Private directory access rules must be effective and inheritable.")
			}
		}
		as := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if as.Equals(sid) {
			seenOwner = true
		} else if !as.Equals(system) && !as.Equals(admins) {
			return errors.New("Private state grants access to another Windows principal.")
		}
	}
	return errors.New("Private state owner access rule is missing.")
}
