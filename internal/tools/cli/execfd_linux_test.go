//go:build linux

package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecViaFD_SetsExtraFilesAndProcSelfPath(t *testing.T) {
	execCmd := exec.Command("/bin/sh")

	f, err := execViaFD(execCmd, "/bin/sh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f == nil {
		t.Fatal("expected a non-nil file")
	}
	defer f.Close()

	if len(execCmd.ExtraFiles) != 1 || execCmd.ExtraFiles[0] != f {
		t.Errorf("expected ExtraFiles=[f], got %v", execCmd.ExtraFiles)
	}
	if execCmd.Path != "/proc/self/fd/3" {
		t.Errorf("expected Path=/proc/self/fd/3, got %q", execCmd.Path)
	}
}

func TestExecViaFD_DetectsSelfBinary(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable failed: %v", err)
	}

	execCmd := exec.CommandContext(context.Background(), self)
	f, err := execViaFD(execCmd, self)
	if err == nil {
		t.Fatal("expected an error for a self-binary")
	}
	if f != nil {
		t.Error("expected a nil file when self-invocation is detected")
	}
	var se *SecurityError
	if !errors.As(err, &se) {
		t.Errorf("expected *SecurityError, got %T: %v", err, err)
	}
}

func TestExecViaFD_ImmuneToPathSwapAfterOpen(t *testing.T) {
	// Proves the actual security property: once execViaFD has opened the
	// resolved file, replacing what the ORIGINAL PATH points to has no
	// effect on what gets executed — the exec runs the exact content that
	// was already open, via /proc/self/fd, not whatever a later attacker
	// swapped the path to. This is what fully closes the check-then-exec
	// race on Linux (a path-only check-then-exec cannot make this
	// guarantee, no matter how tight the window between them).
	dir := t.TempDir()
	target := filepath.Join(dir, "tool")
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho ORIGINAL-MARKER\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	execCmd := exec.CommandContext(context.Background(), target)
	f, err := execViaFD(execCmd, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer f.Close()

	// Simulate an attacker winning the race after execViaFD's internal
	// open but before the process actually runs: replace the path's
	// content entirely.
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho SWAPPED-MARKER\n"), 0o755); err != nil {
		t.Fatal(err)
	}

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

func TestExecViaFD_OpenFailure_FailsClosedRatherThanFallingBack(t *testing.T) {
	// MAJOR finding from automated review: an earlier version
	// returned (nil, nil) when the O_PATH open itself failed, which the
	// caller in tool.go treats as "no hardening available, proceed with
	// the plain path-based exec already set up" — silently reintroducing
	// the check-then-exec race this whole mechanism exists to close,
	// exactly in the case (an unexpected open failure on an
	// already-validated path) an attacker racing that path would produce.
	// execViaFD must fail closed (return a non-nil error) instead, so the
	// caller aborts the command rather than falling back unprotected.
	execCmd := exec.Command("/nonexistent/does-not-exist")

	f, err := execViaFD(execCmd, "/nonexistent/does-not-exist")
	if err == nil {
		t.Fatal("expected an error when the O_PATH open fails, got nil")
	}
	if f != nil {
		t.Errorf("expected a nil file on open failure, got %v", f)
	}
	var se *SecurityError
	if errors.As(err, &se) {
		t.Errorf("expected a plain open error, not a SecurityError: %v", err)
	}
}

func TestExecViaFD_ExecuteOnlyPermission_StillWorks(t *testing.T) {
	// Found by review right after the first version of this fix landed:
	// a plain os.Open requires read permission, but exec only requires
	// execute permission — a legitimately execute-only file (mode 0111,
	// no read bit) would wrongly fail. O_PATH requires neither.
	//
	// Only a compiled binary can actually run execute-only: the kernel's
	// ELF loader maps it via mmap(PROT_EXEC), no read() call needed. A
	// script cannot — its interpreter has to read the script text itself,
	// which needs read permission no matter how it was exec'd. So this
	// test copies a real system binary rather than writing a script
	// (confirmed against a shell-script fixture during development: that
	// version failed even with a correct fix, for exactly this reason —
	// not a bug in execViaFD, a wrong choice of test fixture).
	if os.Geteuid() == 0 {
		t.Skip("running as root — permission bits don't restrict access, can't exercise this case")
	}

	src, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("no 'true' binary on PATH to copy: %v", err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("cannot read %q to copy it: %v", src, err)
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "tool")
	if err := os.WriteFile(target, data, 0o111); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(target); err == nil {
		t.Skip("file is readable despite mode 0111 in this environment — can't exercise this case")
	}

	execCmd := exec.CommandContext(context.Background(), target)
	f, err := execViaFD(execCmd, target)
	if err != nil {
		t.Fatalf("execViaFD returned an error for a legitimately execute-only file: %v", err)
	}
	if f == nil {
		t.Fatal("expected a non-nil file for a successfully-opened, non-self executable")
	}
	defer f.Close()

	if err := execCmd.Run(); err != nil {
		t.Fatalf("exec failed: %v", err)
	}
}
