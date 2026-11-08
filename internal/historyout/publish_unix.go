//go:build linux || darwin

package historyout

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"golang.org/x/sys/unix"
)

type nativeRoot struct {
	state, exports         *os.File
	statePath, exportsPath string
}

func openDirectoryAt(parent *os.File, name, path string) (*os.File, error) {
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if err = localstate.VerifyPrivateFile(f, true); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%w: private directory", ErrUnsupported)
	}
	return f, nil
}
func openNativeRoot(path string) (nativeRoot, RootIdentity, error) {
	var n nativeRoot
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return n, RootIdentity{}, err
	}
	n.statePath = path
	n.exportsPath = path
	n.state = os.NewFile(uintptr(fd), path)
	if err = localstate.VerifyPrivateFile(n.state, true); err != nil {
		_ = n.state.Close()
		return nativeRoot{}, RootIdentity{}, fmt.Errorf("%w: state root", ErrUnsupported)
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
func statIdentity(f *os.File) (string, string, error) {
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return "", "", err
	}
	volume, err := nativeVolume(f)
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("%x:%x:%s", uint64(st.Dev), uint64(st.Ino), volume), volume, nil
}
func sameNamed(parent *os.File, name string, held *os.File, directory bool) error {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if directory {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Openat(int(parent.Fd()), name, flags, 0)
	if err != nil {
		return ErrConflict
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	a, e1 := f.Stat()
	b, e2 := held.Stat()
	if e1 != nil || e2 != nil || !os.SameFile(a, b) {
		return ErrConflict
	}
	av, e1 := nativeVolume(f)
	bv, e2 := nativeVolume(held)
	if e1 != nil || e2 != nil || av != bv {
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

	state, sv, err := statIdentity(n.state)
	if err != nil {
		return RootIdentity{}, err
	}
	out, ov, err := statIdentity(n.exports)
	if err != nil {
		return RootIdentity{}, err
	}
	if sv != ov {
		return RootIdentity{}, fmt.Errorf("%w: export root crosses mount", ErrUnsupported)
	}
	return RootIdentity{StateRoot: state, ExportRoot: out}, nil
}
func (n nativeRoot) requireAbsent(name string) error {
	var st unix.Stat_t
	err := unix.Fstatat(int(n.exports.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err == nil {
		return ErrConflict
	}
	return err
}
func (n nativeRoot) publish(ctx context.Context, name string, payload []byte, result Result, hook func(string) error) (out Result, outErr error) {
	fd, err := unix.Openat(int(n.exports.Fd()), name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		if errors.Is(err, unix.EEXIST) {
			return result, ErrConflict
		}
		return result, err
	}
	f := os.NewFile(uintptr(fd), filepath.Join(n.exportsPath, name))
	result.Created = true
	defer func() { outErr = errors.Join(outErr, f.Close()) }()
	if hook != nil {
		if err = hook("after_create"); err != nil {
			return result, err
		}
	}
	if err = unix.Fchmod(fd, 0600); err != nil {
		return result, err
	}
	if err = localstate.VerifyPrivateFile(f, false); err != nil {
		return result, ErrUnsupported
	}
	_, parentVolume, err := statIdentity(n.exports)
	if err != nil {
		return result, err
	}
	fileID, fileVolume, err := statIdentity(f)
	if err != nil || fileVolume != parentVolume {
		return result, ErrUnsupported
	}
	result.FileIdentity = fileID
	for result.BytesWritten < len(payload) {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		end := result.BytesWritten + 65536
		if end > len(payload) {
			end = len(payload)
		}
		n, writeErr := f.Write(payload[result.BytesWritten:end])
		result.BytesWritten += n
		if writeErr != nil {
			return result, writeErr
		}
		if n == 0 {
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
	info, err := f.Stat()
	if err != nil || info.Size() != int64(len(payload)) {
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
	if err = sameNamed(n.exports, name, f, false); err != nil {
		return result, err
	}
	if _, err = n.identity(); err != nil {
		return result, err
	}
	if hook != nil {
		if err = hook("before_directory_sync"); err != nil {
			return result, err
		}
	}
	if err = unix.Fsync(int(n.exports.Fd())); err != nil {
		return result, err
	}
	result.DirectorySync = "synced"
	if err = sameNamed(n.exports, name, f, false); err != nil {
		return result, err
	}
	if _, err = n.identity(); err != nil {
		return result, err
	}
	result.NameVerified = true
	return result, nil
}
