//go:build windows

package lsp

import "os/exec"

// Windows has no process groups here, so only the server process itself is killed.
// shortcut: grandchildren of the server may survive Close, add a Job Object to bound them.
func setProcessGroup(cmd *exec.Cmd) {}

func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
