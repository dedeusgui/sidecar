//go:build windows

package app

import (
	"os"
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

// SpawnIndexer starts `sidecar index` detached from the hook's console.
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
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
