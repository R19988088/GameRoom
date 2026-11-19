//go:build !windows

package launch

import (
	"os/exec"
	"syscall"
)

// applySysProcAttr gives the emulator its own process group, so closing
// the launcher does not take it down with a signal meant for us.
func applySysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
