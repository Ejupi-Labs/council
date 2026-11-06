//go:build linux

package workspace

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// os.Root does not constrain bind mounts. Reject nested mountpoints, including
// same-device bind mounts, before copying any body from a Linux source tree.
func validateSourceLayout(repo string) error {
	f, e := os.Open("/proc/self/mountinfo")
	if e != nil {
		return fmt.Errorf("%w Source mount layout cannot be verified.", ErrBlocked)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 1<<20)
	decode := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	for s.Scan() {
		parts := strings.Fields(s.Text())
		if len(parts) < 6 {
			return fmt.Errorf("%w Source mount layout is unsupported.", ErrBlocked)
		}
		mount := filepath.Clean(decode.Replace(parts[4]))
		if mount != repo && within(repo, mount) {
			return fmt.Errorf("%w Nested source mounts cannot be captured safely.", ErrBlocked)
		}
	}
	if s.Err() != nil {
		return fmt.Errorf("%w Source mount layout exceeds its observation limit.", ErrBlocked)
	}
	return nil
}
