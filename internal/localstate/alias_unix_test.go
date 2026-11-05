//go:build darwin || linux

package localstate

import "os"

func createAncestorAlias(target, alias string) error { return os.Symlink(target, alias) }
