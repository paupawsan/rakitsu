//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// setNewProcessGroup configures cmd to start in its own process group, so
// killProcessGroup can terminate the whole tree (e.g. a `sh -c` wrapper and
// whatever it spawned) instead of only the direct child.
func setNewProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup kills the process group started by setNewProcessGroup.
// Best-effort: called from Cmd.Cancel, whose error only affects whether the
// returned error looks like a cancellation or a normal exit, so any failure
// here (e.g. the group already exited) is safe to ignore upstream.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
