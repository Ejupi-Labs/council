//go:build !windows

package workspace

import "os/exec"

func configureGitCommand(_ *exec.Cmd) {}
