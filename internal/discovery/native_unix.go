//go:build darwin || linux

package discovery

import (
	"os"
	"path/filepath"
)

// Inspect only the documented native Claude launcher in addition to PATH. No
// vendor configuration, authentication tree or launcher contents are read.
func nativeCandidates(id string) ([]string, error) {
	if id != "claude-code" {
		return nil, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(home) {
		return nil, nil
	}
	return []string{filepath.Join(home, ".local", "bin", "claude")}, nil
}
