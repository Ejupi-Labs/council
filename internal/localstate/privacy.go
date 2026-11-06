package localstate

import (
	"errors"
	"os"
)

// checkPrivateObject checks the retained physical object before permissions may
// change. Ordinary private files must have exactly one directory entry so two
// state roots cannot acquire separate locks for the same database or outside file.
func checkPrivateObject(f *os.File, dir bool) error {
	i, err := f.Stat()
	if err != nil {
		return err
	}
	if i.IsDir() != dir || (!dir && !i.Mode().IsRegular()) {
		return errors.New("Private state must be an ordinary owned file or directory.")
	}
	if !dir {
		return verifySingleLink(f)
	}
	return nil
}

func checkPrivatePath(f *os.File, dir bool) error {
	if err := checkPrivateObject(f, dir); err != nil {
		return err
	}
	i, err := f.Stat()
	if err != nil {
		return err
	}
	pathInfo, err := os.Lstat(f.Name())
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || pathInfo.IsDir() != dir || (!dir && !pathInfo.Mode().IsRegular()) || !os.SameFile(i, pathInfo) {
		return errors.New("Private state changed before protection.")
	}
	return nil
}

// SyncPrivateDirectory publishes newly created private entries durably. Callers
// must already own and validate the directory through the state root.
func SyncPrivateDirectory(path string) error { return syncDirectory(path) }
