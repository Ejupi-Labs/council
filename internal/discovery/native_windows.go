//go:build windows

package discovery

import (
	"errors"
	"golang.org/x/sys/windows/registry"
	"strings"
)

// App Paths is Windows installation registration, not vendor authentication/configuration.
// Only these exact executable registrations and their default path values are inspected.
func nativeCandidates(id string) ([]string, error) {
	name := ""
	for _, h := range harnesses {
		if h.id == id {
			name = h.name + ".exe"
			break
		}
	}
	if name == "" {
		return nil, nil
	}
	var paths []string
	var scanErr error
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		key, err := registry.OpenKey(root, `Software\Microsoft\Windows\CurrentVersion\App Paths\`+name, registry.QUERY_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			scanErr = err
			continue
		}
		value, kind, err := key.GetStringValue("")
		key.Close()
		if err != nil {
			scanErr = err
			continue
		}
		if kind == registry.EXPAND_SZ {
			value, err = registry.ExpandString(value)
			if err != nil {
				scanErr = err
				continue
			}
		}
		if value != "" {
			paths = append(paths, strings.Trim(value, `"`))
		}
	}
	return paths, scanErr
}
