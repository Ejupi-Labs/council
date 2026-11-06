//go:build windows

package workspace

import (
	"os/exec"
	"syscall"
)

// Source/index operations use captured pipes, not a console window. Avoid
// console allocation when launched by Council's detached continuing owner.
func configureGitCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
