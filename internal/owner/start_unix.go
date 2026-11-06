//go:build !windows

package owner

import (
	"os/exec"
	"syscall"
)

func configureBackground(command *exec.Cmd) { command.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
