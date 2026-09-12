//go:build !linux

package cli

import (
	"os/exec"
	"testing"
)

func TestExecViaFD_NoOpOnNonLinux(t *testing.T) {
	execCmd := exec.Command("/bin/echo")
	originalPath := execCmd.Path

	f, err := execViaFD(execCmd, "/bin/echo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != nil {
		t.Errorf("expected a nil file on non-Linux, got %v", f)
	}
	if execCmd.Path != originalPath {
		t.Errorf("expected Path unchanged on non-Linux (no /proc), got %q (was %q)", execCmd.Path, originalPath)
	}
	if len(execCmd.ExtraFiles) != 0 {
		t.Errorf("expected no ExtraFiles set on non-Linux, got %v", execCmd.ExtraFiles)
	}
}
