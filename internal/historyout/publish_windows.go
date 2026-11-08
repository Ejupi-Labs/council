//go:build windows

package historyout

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"golang.org/x/sys/windows"
)

type nativeRoot struct {
	state, exports         *os.File
	statePath, exportsPath string
}

func openStateDirectory(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.FILE_LIST_DIRECTORY|windows.FILE_TRAVERSE|windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL|windows.SYNCHRONIZE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	if err = localstate.VerifyPrivateFile(f, true); err != nil {
		_ = f.Close()
		return nil, ErrUnsupported
	}
	return f, nil
}
func openRelative(parent *os.File, name, path string, directory, create bool) (*os.File, error) {
	value, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attrs := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: windows.Handle(parent.Fd()), ObjectName: value, Attributes: windows.OBJ_DONT_REPARSE | windows.OBJ_CASE_INSENSITIVE}
	access := uint32(windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL | windows.SYNCHRONIZE)
	options := uint32(windows.FILE_NON_DIRECTORY_FILE | windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT)
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE)
	disposition := uint32(windows.FILE_OPEN)
	if directory {
		access |= windows.FILE_LIST_DIRECTORY | windows.FILE_TRAVERSE | windows.FILE_WRITE_DATA
		options = windows.FILE_DIRECTORY_FILE | windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT
	}
	if create {
		token, tokenErr := windows.GetCurrentProcessToken().GetTokenUser()
		if tokenErr != nil {
			return nil, tokenErr
		}
		sid := token.User.Sid
		if sid == nil || !sid.IsValid() {
			return nil, ErrUnsupported
		}
		sd, secErr := windows.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;;FA;;;" + sid.String() + ")(A;;FA;;;SY)(A;;FA;;;BA)")
		if secErr != nil {
			return nil, secErr
		}
		attrs.SecurityDescriptor = sd
		access = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.READ_CONTROL | windows.SYNCHRONIZE
		disposition = windows.FILE_CREATE
		share = windows.FILE_SHARE_READ
	}
	var h windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&h, access, &attrs, &status, nil, windows.FILE_ATTRIBUTE_NORMAL, share, disposition, options, 0, 0)
	if err != nil {
		if status, ok := err.(windows.NTStatus); ok {
			return nil, status.Errno()
		}
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	if !create {
		if err = localstate.VerifyPrivateFile(f, directory); err != nil {
			_ = f.Close()
			return nil, ErrUnsupported
		}
	}
	return f, nil
}
func winIdentity(f *os.File) (string, uint32, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return "", 0, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", 0, ErrUnsupported
	}
	return fmt.Sprintf("%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), info.VolumeSerialNumber, nil
}
func openNativeRoot(path string) (nativeRoot, RootIdentity, error) {
	n := nativeRoot{statePath: path, exportsPath: path}
	var err error
	n.state, err = openStateDirectory(path)
	if err != nil {
		return nativeRoot{}, RootIdentity{}, err
	}
	n.exports = n.state
	id, err := n.identity()
	if err != nil {
		_ = n.close()
		return nativeRoot{}, RootIdentity{}, err
	}
	return n, id, nil
}
func (n nativeRoot) close() error { return n.state.Close() }
func sameNamed(parent *os.File, name, path string, held *os.File, directory bool) error {
	f, err := openRelative(parent, name, path, directory, false)
	if err != nil {
		return ErrConflict
	}
	defer f.Close()
	a, av, e1 := winIdentity(f)
	b, bv, e2 := winIdentity(held)
	if e1 != nil || e2 != nil || a != b || av != bv {
		return ErrConflict
	}
	return nil
}
func (n nativeRoot) identity() (RootIdentity, error) {
	if err := localstate.VerifyPrivateFile(n.state, true); err != nil {
		return RootIdentity{}, ErrUnsupported
	}
	if err := localstate.VerifyPrivateFile(n.exports, true); err != nil {
		return RootIdentity{}, ErrUnsupported
	}
	current, err := os.Lstat(n.statePath)
	if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 {
		return RootIdentity{}, ErrConflict
	}
	held, err := n.state.Stat()
	if err != nil || !os.SameFile(current, held) {
		return RootIdentity{}, ErrConflict
	}

	state, sv, err := winIdentity(n.state)
	if err != nil {
		return RootIdentity{}, err
	}
	output, ov, err := winIdentity(n.exports)
	if err != nil {
		return RootIdentity{}, err
	}
	if sv != ov {
		return RootIdentity{}, ErrUnsupported
	}
	return RootIdentity{StateRoot: state, ExportRoot: output}, nil
}
func (n nativeRoot) requireAbsent(name string) error {
	f, err := openRelative(n.exports, name, filepath.Join(n.exportsPath, name), false, false)
	if err == nil {
		_ = f.Close()
		return ErrConflict
	}
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return nil
	}
	return err
}
func (n nativeRoot) publish(ctx context.Context, name string, payload []byte, result Result, hook func(string) error) (out Result, outErr error) {
	f, err := openRelative(n.exports, name, filepath.Join(n.exportsPath, name), false, true)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return result, ErrConflict
		}
		return result, err
	}
	result.Created = true
	defer func() { outErr = errors.Join(outErr, f.Close()) }()
	if hook != nil {
		if err = hook("after_create"); err != nil {
			return result, err
		}
	}
	if err = localstate.VerifyPrivateFile(f, false); err != nil {
		return result, ErrUnsupported
	}
	id, vol, err := winIdentity(f)
	if err != nil {
		return result, err
	}
	_, parentVolume, err := winIdentity(n.exports)
	if err != nil || vol != parentVolume {
		return result, ErrUnsupported
	}
	result.FileIdentity = id
	for result.BytesWritten < len(payload) {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		end := result.BytesWritten + 65536
		if end > len(payload) {
			end = len(payload)
		}
		count, writeErr := f.Write(payload[result.BytesWritten:end])
		result.BytesWritten += count
		if writeErr != nil {
			return result, writeErr
		}
		if count == 0 {
			return result, io.ErrShortWrite
		}
		if hook != nil {
			if err = hook("after_write"); err != nil {
				return result, err
			}
		}
	}
	if hook != nil {
		if err = hook("before_file_sync"); err != nil {
			return result, err
		}
	}
	if err = f.Sync(); err != nil {
		return result, err
	}
	result.FileFlushed = true
	stat, err := f.Stat()
	if err != nil || stat.Size() != int64(len(payload)) {
		return result, ErrConflict
	}
	check := make([]byte, len(payload))
	if _, err = f.ReadAt(check, 0); err != nil && !errors.Is(err, io.EOF) {
		return result, err
	}
	if !bytes.Equal(check, payload) {
		return result, ErrConflict
	}
	if err = localstate.VerifyPrivateFile(f, false); err != nil {
		return result, ErrUnsupported
	}
	if hook != nil {
		if err = hook("before_name_verify"); err != nil {
			return result, err
		}
	}
	if err = sameNamed(n.exports, name, filepath.Join(n.exportsPath, name), f, false); err != nil {
		return result, err
	}
	if _, err = n.identity(); err != nil {
		return result, err
	}
	result.NameVerified = true
	if hook != nil {
		if err = hook("before_directory_sync"); err != nil {
			return result, err
		}
	}
	result.DirectorySync = "unavailable" // No ordinary Windows directory flush guarantee.
	return result, nil
}
