//go:build windows

// Package localpath resolves physical paths using filesystem metadata only.
package localpath

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

// Resolve uses the final handle path so directory junction ancestors, symbolic links,
// and case/short-name aliases receive one physical spelling without reading contents.
func Resolve(path string) (string, error) {
	// Resolve ordinary symbolic links in Go before the handle lookup. Junctions can
	// be ModeIrregular in Go 1.27; the handle resolves those when EvalSymlinks cannot.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buffer := make([]uint16, 260)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buffer[0], uint32(len(buffer)), 0)
		if err != nil {
			return "", err
		}
		if n >= uint32(len(buffer)) {
			if n >= 32768 {
				return "", errors.New("The path exceeds the Windows path limit.")
			}
			buffer = make([]uint16, n+1)
			continue
		}
		resolved, err := decodePath(buffer[:n])
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(resolved, `\\?\UNC\`) {
			return `\\` + strings.TrimPrefix(resolved, `\\?\UNC\`), nil
		}
		return strings.TrimPrefix(resolved, `\\?\`), nil
	}
}

func decodePath(units []uint16) (string, error) {
	decoded := windows.UTF16ToString(units)
	roundTrip, err := windows.UTF16FromString(decoded)
	if err != nil || !utf8.ValidString(decoded) || len(units) == 0 || len(roundTrip) != len(units)+1 {
		return "", errors.New("The physical path cannot be represented losslessly.")
	}
	for i, unit := range units {
		if roundTrip[i] != unit {
			return "", errors.New("The physical path cannot be represented losslessly.")
		}
	}
	return decoded, nil
}
