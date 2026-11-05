//go:build !windows

package app

import (
	"os"
	"os/exec"
	"syscall"
)

// SpawnIndexer starts `sidecar index` for a worktree in its own session, so it outlives
// the hook process and is not killed with the agent's terminal.
func SpawnIndexer(root string, history bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"index", "--root", root}
	if history {
		args = append(args, "--history")
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
