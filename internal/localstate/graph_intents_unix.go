//go:build darwin || linux

package localstate

import "os"

func graphIntentOpenRead(path string, dir bool) (*os.File, error) {
	return openPrivateNoFollow(path, dir)
}
func graphIntentOpenLock(path string) (*os.File, error) { return openPrivateNoFollow(path, false) }
