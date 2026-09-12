//go:build !linux

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExecViaFD_NoOpOnNonLinux(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	execCmd := exec.Command("/bin/echo")
	originalPath := execCmd.Path

	execViaFD(execCmd, f)

	if execCmd.Path != originalPath {
		t.Errorf("expected Path unchanged on non-Linux (no /proc), got %q (was %q)", execCmd.Path, originalPath)
	}
	if len(execCmd.ExtraFiles) != 0 {
		t.Errorf("expected no ExtraFiles set on non-Linux, got %v", execCmd.ExtraFiles)
	}
}
