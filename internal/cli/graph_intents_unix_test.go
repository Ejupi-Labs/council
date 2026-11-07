//go:build darwin || linux

package cli

import "os"

func graphIntentTestAlias(target, alias string) error { return os.Symlink(target, alias) }
func graphIntentTestMakePublic(path string) error     { return os.Chmod(path, 0644) }
