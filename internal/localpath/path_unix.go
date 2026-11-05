//go:build darwin || linux

// Package localpath resolves physical paths using filesystem metadata only.
package localpath

import "path/filepath"

func Resolve(path string) (string, error) { return filepath.EvalSymlinks(path) }
