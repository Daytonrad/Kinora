//go:build windows

package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const (
	createNewProcessGroup  = 0x00000200
	detachedProcess        = 0x00000008
	createBreakawayFromJob = 0x01000000
)

// startDetached runs update.ps1 in its own process, outside the service's process tree, so it keeps running
// while it stops and restarts the Kinora service.
func startDetached(script string) error {
	ps := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(ps); err != nil {
		ps = "powershell.exe"
	}
	start := func(flags uint32) error {
		cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
		cmd.Dir = filepath.Dir(script)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
	base := uint32(createNewProcessGroup | detachedProcess)
	// leave the service's job object when there is one; a job that forbids breaking away refuses the flag
	if err := start(base | createBreakawayFromJob); err == nil {
		return nil
	}
	return start(base)
}
