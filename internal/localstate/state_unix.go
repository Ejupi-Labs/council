//go:build darwin || linux

package localstate

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func securePrivate(path string, dir bool) error {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return err
	}
	if st.Uid != uint32(os.Getuid()) {
		return errors.New("Private state belongs to another user.")
	}
	mode := os.FileMode(0600)
	if dir {
		mode = 0700
	}
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	return verifyPrivate(path, dir)
}
func verifyPrivate(path string, dir bool) error {
	i, err := os.Lstat(path)
	if err != nil {
		return err
	}
	want := os.FileMode(0600)
	if dir {
		want = 0700
	}
	if i.Mode()&os.ModeSymlink != 0 || i.IsDir() != dir || (!dir && !i.Mode().IsRegular()) || i.Mode().Perm() != want {
		return errors.New("Private state permissions are invalid.")
	}
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return err
	}
	if st.Uid != uint32(os.Getuid()) {
		return errors.New("Private state belongs to another user.")
	}
	return nil
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
