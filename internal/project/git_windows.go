//go:build windows

package project

import (
	"os/exec"
	"syscall"
)

// Metadata uses pipes and never requests an interactive console, including when
// the continuing owner itself was started detached from its caller's console.
func configureGitCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
