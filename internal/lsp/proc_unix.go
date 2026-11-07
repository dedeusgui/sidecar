//go:build unix

package lsp

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the server in its own group so Close also stops the children it spawns
// (gopls runs `go` subprocesses, for example).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessTree(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
