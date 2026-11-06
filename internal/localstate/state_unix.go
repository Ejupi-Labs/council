//go:build darwin || linux

package localstate

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func securePrivate(path string, dir bool) error {
	f, err := openPrivateNoFollow(path, dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return ProtectPrivateFile(f, dir)
}
func verifyPrivate(path string, dir bool) error {
	f, err := openPrivateNoFollow(path, dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return VerifyPrivateFile(f, dir)
}
func openPrivateNoFollow(path string, dir bool) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
	if dir {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Open(path, flags, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if err := checkPrivatePath(f, dir); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
func lockNative(f *os.File) error   { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
func unlockNative(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }
func isLockBusy(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}
func atomicReplace(from, to string) error { return os.Rename(from, to) }
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
