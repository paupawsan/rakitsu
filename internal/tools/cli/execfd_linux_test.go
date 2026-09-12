//go:build linux

package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecViaFD_SetsExtraFilesAndProcSelfPath(t *testing.T) {
	f, err := os.Open("/bin/sh")
	if err != nil {
		t.Skipf("no /bin/sh available: %v", err)
	}
	defer f.Close()

	execCmd := exec.Command("/bin/sh")
	execViaFD(execCmd, f)

	if len(execCmd.ExtraFiles) != 1 || execCmd.ExtraFiles[0] != f {
		t.Errorf("expected ExtraFiles=[f], got %v", execCmd.ExtraFiles)
	}
	if execCmd.Path != "/proc/self/fd/3" {
		t.Errorf("expected Path=/proc/self/fd/3, got %q", execCmd.Path)
	}
}

func TestExecViaFD_ImmuneToPathSwapAfterOpen(t *testing.T) {
	// Proves the actual security property: once a file is opened and
	// handed to execViaFD, replacing what the ORIGINAL PATH points to has
	// no effect on what gets executed — the exec runs the exact content
	// that was already open, via /proc/self/fd, not whatever a later
	// attacker swapped the path to. This is what fully closes the
	// check-then-exec race on Linux (a path-only check-then-exec cannot
	// make this guarantee, no matter how tight the window between them).
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho ORIGINAL-MARKER\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(target)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer f.Close()

	// Simulate an attacker winning the race after the identity check/open
	// but before exec: replace the path's content entirely.
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho SWAPPED-MARKER\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	execCmd := exec.CommandContext(context.Background(), target)
	execViaFD(execCmd, f)

	out, err := execCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("exec failed: %v (output: %s)", err, out)
	}
	if !strings.Contains(string(out), "ORIGINAL-MARKER") {
		t.Errorf("expected the originally-opened file's output, got %q — the swap defeated the fd-based exec", out)
	}
	if strings.Contains(string(out), "SWAPPED-MARKER") {
		t.Error("executed the swapped-in file instead of the originally-verified one — the race was not closed")
	}
}
