//go:build windows

package owner

import (
	"os/exec"
	"syscall"
)

func configureBackground(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008 | 0x00000200}
}
