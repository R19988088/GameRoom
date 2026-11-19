//go:build windows

package launch

import (
	"os/exec"
	"syscall"
)

// applySysProcAttr keeps the emulator from opening a console window.
func applySysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
