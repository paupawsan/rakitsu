//go:build windows

package cli

import "os/exec"

// setNewProcessGroup is a no-op on Windows. Process groups there are a
// different primitive (job objects, not POSIX pgid) and aren't implemented
// here — a timed-out command's grandchildren can still be left running,
// same as before this fix. exec.Cmd's default Cancel behavior (kill the
// direct child) still applies.
func setNewProcessGroup(cmd *exec.Cmd) {}

// killProcessGroup is a no-op on Windows; see setNewProcessGroup.
func killProcessGroup(cmd *exec.Cmd) error { return nil }
