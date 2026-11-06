//go:build windows

package workspace

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

func applyOpenDirectory(path string) (*os.File, error) {
	if !filepath.IsAbs(path) || !utf8.ValidString(path) || strings.ContainsRune(path, 0) {
		return nil, ErrApplyUnsupported
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.FILE_LIST_DIRECTORY|windows.FILE_TRAVERSE|windows.FILE_WRITE_DATA|windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL|windows.SYNCHRONIZE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if _, err = applyNativeInfo(file, true); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func applyOpenRelative(parent *os.File, name string, create, directory bool) (*os.File, error) {
	if parent == nil || name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\:\x00") || !utf8.ValidString(name) {
		return nil, ErrApplyUnsupported
	}
	value, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: windows.Handle(parent.Fd()), ObjectName: value, Attributes: windows.OBJ_DONT_REPARSE}
	access := uint32(windows.FILE_GENERIC_READ | windows.READ_CONTROL | windows.DELETE)
	options := uint32(windows.FILE_NON_DIRECTORY_FILE | windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_NO_EA_KNOWLEDGE)
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE)
	disposition := uint32(windows.FILE_OPEN)
	if directory {
		if create {
			return nil, ErrApplyUnsupported
		}
		access = windows.FILE_LIST_DIRECTORY | windows.FILE_TRAVERSE | windows.FILE_WRITE_DATA | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL | windows.SYNCHRONIZE
		options = windows.FILE_DIRECTORY_FILE | windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT
	}
	if create {
		access |= windows.FILE_GENERIC_WRITE | windows.WRITE_DAC | windows.WRITE_OWNER
		disposition = windows.FILE_CREATE
		share = 0
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	if err = windows.NtCreateFile(&handle, access, &attributes, &status, nil, windows.FILE_ATTRIBUTE_NORMAL, share, disposition, options, 0, 0); err != nil {
		if status, ok := err.(windows.NTStatus); ok {
			err = status.Errno()
		}
		return nil, err
	}
	file := os.NewFile(uintptr(handle), name)
	if _, err = applyNativeInfo(file, directory); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func applyNativeInfo(file *os.File, directory bool) (applyFileInfo, error) {
	var out applyFileInfo
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return out, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
		return out, ErrApplyUnsupported
	}
	allowed := uint32(windows.FILE_ATTRIBUTE_NORMAL | windows.FILE_ATTRIBUTE_ARCHIVE)
	if directory {
		allowed |= windows.FILE_ATTRIBUTE_DIRECTORY | windows.FILE_ATTRIBUTE_READONLY
	}
	if info.FileAttributes & ^allowed != 0 {
		return out, ErrApplyUnsupported
	}
	if err := applyNoExtendedAttributes(file); err != nil {
		return out, err
	}
	if err := applyNoIntegrityLabel(file); err != nil {
		return out, err
	}
	if !directory {
		if err := applyOrdinaryStreams(file); err != nil {
			return out, err
		}
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return out, err
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return out, ErrApplyUnsupported
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil || dacl.AceCount == 0 || dacl.AceCount > 256 {
		return out, ErrApplyUnsupported
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(dacl, i, &ace); err != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE && ace.Header.AceType != windows.ACCESS_DENIED_ACE_TYPE {
			return out, ErrApplyUnsupported
		}
	}
	control, _, err := sd.Control()
	if err != nil {
		return out, err
	}
	stat, err := file.Stat()
	if err != nil {
		return out, err
	}
	security := sd.String()
	start := strings.Index(security, "D:")
	if start < 0 {
		return out, ErrApplyUnsupported
	}
	security = "O:" + owner.String() + security[start:]
	out = applyFileInfo{Identity: fmt.Sprintf("windows:%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), Volume: fmt.Sprintf("%08x", info.VolumeSerialNumber), Owner: owner.String(), Mode: uint32(stat.Mode().Perm()), Security: security, Control: uint32(control), Native: sd}
	return out, nil
}

// FILE_NO_EA_KNOWLEDGE rejects only FILE_NEED_EA. Ordinary optional EAs also
// carry user metadata, so every checked object needs a descriptor-bound absence
// result before staging metadata or moving a destination.
func applyNoExtendedAttributes(file *os.File) error {
	size := ^uint32(0)
	var status windows.IO_STATUS_BLOCK
	err := windows.NtQueryInformationFile(windows.Handle(file.Fd()), &status, (*byte)(unsafe.Pointer(&size)), uint32(unsafe.Sizeof(size)), windows.FileEaInformation)
	if err != nil || status.Information != unsafe.Sizeof(size) {
		return fmt.Errorf("%w Extended attribute metadata is unavailable: %v", ErrApplyUnsupported, err)
	}
	if size != 0 {
		return fmt.Errorf("%w Extended attributes cannot be preserved.", ErrApplyUnsupported)
	}
	return nil
}

// LABEL_SECURITY_INFORMATION is queryable with READ_CONTROL. This bounded
// copier does not set labels or request privileged SACL access; an explicit
// integrity label on a file, parent or stage requires manual application.
func applyNoIntegrityLabel(file *os.File) error {
	descriptor, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("%w Mandatory integrity metadata is unavailable: %v", ErrApplyUnsupported, err)
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return fmt.Errorf("%w Mandatory integrity metadata is unavailable: %v", ErrApplyUnsupported, err)
	}
	// x/sys SACL() reports ERROR_OBJECT_NOT_FOUND when the descriptor has no
	// SACL. A successful label query with SE_SACL_PRESENT clear is positive
	// absence evidence, not an OS query failure to waive.
	if control&windows.SE_SACL_PRESENT == 0 {
		return nil
	}
	label, _, err := descriptor.SACL()
	if err != nil {
		return fmt.Errorf("%w Mandatory integrity metadata is unavailable: %v", ErrApplyUnsupported, err)
	}
	if label != nil && label.AceCount != 0 {
		return fmt.Errorf("%w Explicit mandatory integrity labels cannot be preserved.", ErrApplyUnsupported)
	}
	return nil
}

func applyOrdinaryStreams(file *os.File) error {
	aligned := make([]uint64, (64<<10)/8)
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&aligned[0])), len(aligned)*8)
	err := windows.GetFileInformationByHandleEx(windows.Handle(file.Fd()), windows.FileStreamInfo, &buffer[0], uint32(len(buffer)))
	if err == windows.ERROR_HANDLE_EOF {
		stat, e := file.Stat()
		if e == nil && stat.Size() == 0 {
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("%w Alternate stream metadata is unavailable: %v", ErrApplyUnsupported, err)
	}
	count := 0
	for offset := 0; ; {
		if offset+24 > len(buffer) {
			return ErrApplyUnsupported
		}
		next := int(binary.LittleEndian.Uint32(buffer[offset : offset+4]))
		length := int(binary.LittleEndian.Uint32(buffer[offset+4 : offset+8]))
		if length%2 != 0 || length < 2 || length > len(buffer)-offset-24 {
			return ErrApplyUnsupported
		}
		encoded := make([]uint16, length/2)
		for i := range encoded {
			encoded[i] = binary.LittleEndian.Uint16(buffer[offset+24+2*i : offset+26+2*i])
		}
		name := string(utf16.Decode(encoded))
		if name != "::$DATA" {
			return ErrApplyUnsupported
		}
		count++
		if count > 1 {
			return ErrApplyUnsupported
		}
		if next == 0 {
			break
		}
		if next%8 != 0 || next < 24+length || next > len(buffer)-offset {
			return ErrApplyUnsupported
		}
		offset += next
	}
	return nil
}

func applySecurityParts(sd *windows.SECURITY_DESCRIPTOR) (string, string, error) {
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return "", "", ErrApplyUnsupported
	}
	text := sd.String()
	start := strings.Index(text, "D:")
	if start < 0 {
		return "", "", ErrApplyUnsupported
	}
	dacl := text[start:]
	if end := strings.Index(dacl, "S:"); end >= 0 {
		dacl = dacl[:end]
	}
	return owner.String(), "O:" + owner.String() + dacl, nil
}

func applySetSecurity(stage *os.File, descriptor *windows.SECURITY_DESCRIPTOR, flags windows.SECURITY_INFORMATION) error {
	if _, err := applyNativeInfo(stage, false); err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		return ErrApplyUnsupported
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return ErrApplyUnsupported
	}
	return windows.SetSecurityInfo(windows.Handle(stage.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|flags, owner, nil, dacl, nil)
}

func applyPrepareMetadata(stage *os.File, expected applyFileInfo) error {
	sd, ok := expected.Native.(*windows.SECURITY_DESCRIPTOR)
	if !ok || expected.Mode&^0777 != 0 {
		return ErrApplyUnsupported
	}
	before, err := applyNativeInfo(stage, false)
	if err != nil {
		return err
	}
	if expected.Volume != "" && before.Volume != expected.Volume {
		return ErrApplyUnsupported
	}
	copySD, err := windows.SecurityDescriptorFromString(sd.String())
	if err != nil {
		return err
	}
	dacl, _, err := copySD.DACL()
	if err != nil || dacl == nil {
		return ErrApplyUnsupported
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(dacl, i, &ace); err != nil {
			return err
		}
		ace.Header.AceFlags &^= windows.INHERITED_ACE
	}
	if err = stage.Chmod(os.FileMode(expected.Mode)); err != nil {
		return err
	}
	if err = applySetSecurity(stage, copySD, windows.PROTECTED_DACL_SECURITY_INFORMATION); err != nil {
		return err
	}
	prepared, err := applyNativeInfo(stage, false)
	if err != nil {
		return err
	}
	if prepared.Identity != before.Identity || prepared.Owner != expected.Owner || prepared.Mode != expected.Mode || prepared.Control&uint32(windows.SE_DACL_PROTECTED) == 0 {
		return ErrApplyUnsupported
	}
	_, explicitSecurity, err := applySecurityParts(copySD)
	if err != nil {
		return err
	}
	aces := func(text string) string {
		start := strings.Index(text, "(")
		if start < 0 {
			return ""
		}
		return text[start:]
	}
	if aces(prepared.Security) == "" || aces(prepared.Security) != aces(explicitSecurity) {
		return ErrApplyUnsupported
	}
	if expected.Control&uint32(windows.SE_DACL_PROTECTED) != 0 && !applyMetadataEqual(prepared, expected) {
		return ErrApplyUnsupported
	}
	return nil
}

func applyRestoreMetadata(stage *os.File, expected applyFileInfo) error {
	sd, ok := expected.Native.(*windows.SECURITY_DESCRIPTOR)
	if !ok {
		return ErrApplyUnsupported
	}
	if expected.Control&uint32(windows.SE_DACL_PROTECTED) == 0 {
		if err := applySetSecurity(stage, sd, windows.UNPROTECTED_DACL_SECURITY_INFORMATION); err != nil {
			return err
		}
	}
	after, err := applyNativeInfo(stage, false)
	if err != nil {
		return err
	}
	if !applyMetadataEqual(after, expected) {
		return ErrApplyUnsupported
	}
	return nil
}

var applySecurityDLL = windows.NewLazySystemDLL("advapi32.dll")
var applyCreatePrivateSecurity = applySecurityDLL.NewProc("CreatePrivateObjectSecurityEx")
var applyDestroyPrivateSecurity = applySecurityDLL.NewProc("DestroyPrivateObjectSecurity")

func applyNewFileMetadata(parent *os.File, mode uint32) (applyFileInfo, error) {
	info, err := applyNativeInfo(parent, true)
	if err != nil {
		return applyFileInfo{}, err
	}
	parentSD, ok := info.Native.(*windows.SECURITY_DESCRIPTOR)
	if !ok || mode&^0777 != 0 {
		return applyFileInfo{}, ErrApplyUnsupported
	}
	var token windows.Token
	if err = windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return applyFileInfo{}, err
	}
	defer token.Close()
	type genericMapping struct{ Read, Write, Execute, All uint32 }
	mapping := genericMapping{windows.FILE_GENERIC_READ, windows.FILE_GENERIC_WRITE, windows.FILE_GENERIC_EXECUTE, 0x001f01ff}
	var created *windows.SECURITY_DESCRIPTOR
	success, _, callErr := applyCreatePrivateSecurity.Call(uintptr(unsafe.Pointer(parentSD)), 0, uintptr(unsafe.Pointer(&created)), 0, 0, 1, uintptr(token), uintptr(unsafe.Pointer(&mapping)))
	runtime.KeepAlive(parentSD)
	if success == 0 || created == nil {
		return applyFileInfo{}, fmt.Errorf("%w The new-file destination security rule is unavailable: %v", ErrApplyUnsupported, callErr)
	}
	defer applyDestroyPrivateSecurity.Call(uintptr(unsafe.Pointer(&created)))
	owner, security, err := applySecurityParts(created)
	if err != nil {
		return applyFileInfo{}, err
	}
	// Parse the selected owner/DACL into Go-owned memory. This also drops only
	// transient creator-default flags which are not stored on filesystem objects.
	descriptor, err := windows.SecurityDescriptorFromString(security)
	if err != nil {
		return applyFileInfo{}, err
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return applyFileInfo{}, err
	}
	return applyFileInfo{Volume: info.Volume, Owner: owner, Mode: mode, Security: security, Control: uint32(control), Native: descriptor}, nil
}

func applyRenameExclusive(sourceParent *os.File, sourceName string, source *os.File, destinationParent *os.File, destinationName string) error {
	if source == nil || destinationParent == nil || destinationName == "" || strings.ContainsAny(destinationName, "/\\:\x00") || !utf8.ValidString(destinationName) {
		return ErrApplyUnsupported
	}
	name, err := windows.UTF16FromString(destinationName)
	if err != nil {
		return err
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
	*(*windows.Handle)(unsafe.Pointer(&buffer[unsafe.Offsetof(layout.Root)])) = windows.Handle(destinationParent.Fd())
	*(*uint32)(unsafe.Pointer(&buffer[unsafe.Offsetof(layout.Length)])) = uint32(2 * len(name))
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[offset])), len(name)), name)
	err = windows.NtSetInformationFile(windows.Handle(source.Fd()), &windows.IO_STATUS_BLOCK{}, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation)
	if status, ok := err.(windows.NTStatus); ok {
		return status.Errno()
	}
	return err
}

func applySyncDirectory(parent *os.File) error {
	// Windows does not expose a supported ordinary directory FlushFileBuffers
	// guarantee here. File/journal durability is qualified separately; no universal
	// power-loss namespace transaction is claimed.
	return nil
}
