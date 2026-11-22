//go:build windows

package futek

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// hideConsoleWindow prevents a console window from flashing up when the
// Python helper is launched from the GUI.
func hideConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags = createNoWindow
}
