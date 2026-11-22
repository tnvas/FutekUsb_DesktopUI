//go:build !windows

package futek

import "os/exec"

// hideConsoleWindow is a no-op outside Windows.
func hideConsoleWindow(cmd *exec.Cmd) {}
