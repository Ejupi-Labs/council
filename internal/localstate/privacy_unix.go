//go:build darwin || linux

package localstate

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

// ProtectPrivateFile operates on the opened object, never a path replacement.
func ProtectPrivateFile(f *os.File, dir bool) error {
	if err := checkPrivatePath(f, dir); err != nil {
		return err
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return err
	}
	if st.Uid != uint32(os.Getuid()) {
		return errors.New("Private state belongs to another user.")
	}
	mode := uint32(0600)
	if dir {
		mode = 0700
	}
	if err := unix.Fchmod(int(f.Fd()), mode); err != nil {
		return err
	}
	return VerifyPrivateFile(f, dir)
}

func verifySingleLink(f *os.File) error {
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return err
	}
	if st.Nlink != 1 {
		return errors.New("Private state files must have exactly one hard link.")
	}
	return nil
}

func VerifyPrivateFile(f *os.File, dir bool) error {
	if err := checkPrivateObject(f, dir); err != nil {
		return err
	}
	i, err := f.Stat()
	if err != nil {
		return err
	}
	mode := os.FileMode(0600)
	if dir {
		mode = 0700
	}
	if i.IsDir() != dir || !dir && !i.Mode().IsRegular() || i.Mode().Perm() != mode {
		return errors.New("Private state permissions are invalid.")
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return err
	}
	if st.Uid != uint32(os.Getuid()) {
		return errors.New("Private state belongs to another user.")
	}
	return nil
}
