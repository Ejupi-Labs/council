//go:build !windows

package project

import "os/exec"

func configureGitCommand(_ *exec.Cmd) {}
