//go:build linux

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// oPath is O_PATH's value, the same across every Linux architecture — but
// Go's syscall package only defines the O_PATH constant for some of them
// (arm64, ppc64, s390x, mips64, riscv64, ...), not for 386/amd64. Go's own
// standard-library tests hit this same gap and hardcode the value for the
// identical reason (syscall/exec_linux_test.go). Defining it locally here
// avoids depending on golang.org/x/sys just for one constant.
const oPath = 0x200000

// execViaFD re-execs resolved through a freshly-opened file descriptor
// (via the Linux-only /proc/self/fd magic symlink) instead of by path,
// eliminating the check-then-exec race entirely on this platform: execve
// resolving /proc/self/fd/N loads exactly the inode the fd refers to,
// regardless of anything that happens to the original path afterward —
// the same technique tools like sudo and runc use for equivalent problems.
//
// The fd is opened with O_PATH, not a normal read-oriented open: O_PATH
// requires only that the path is traversable, not that the file itself is
// readable — matching what exec actually needs (execute permission, not
// read permission). A plain os.Open(resolved) would wrongly break a
// legitimately-allowed executable that is execute-only (mode 0111, no
// read bit) — a real configuration this guard must not reject, found by
// review right after this fix's first version landed.
//
// Returns the opened file (with a non-nil error only when the resolved file
// is provably the running rakitsu binary, which must never be exec'd) so the
// caller can close it once the command has finished running.
//
// MAJOR finding from automated review: an earlier version of this
// function returned (nil, nil) — "can't harden further this time, fall back
// to the plain exec-by-path the caller already has set up" — whenever the
// O_PATH open itself failed. That is not a benign fallback: by the time
// execViaFD runs, resolved has already been validated to exist (LookPath +
// Abs + isSelfBinary's own os.Stat in the caller), and O_PATH doesn't even
// need read permission — there is no ordinary reason for the open to fail.
// An attacker racing the resolved path (remove it, or make it momentarily
// untraversable, right after the caller's check) could make exactly this
// open fail, and the silent fallback would then hand the caller a plain
// path-based exec with no fd pinning — reintroducing the very check-then-exec
// race this whole mechanism exists to close, precisely when something is
// already going wrong. Fail closed here instead: propagate the error and let
// the caller abort the command, rather than downgrade to the pre-hardening
// behavior on an unexplained failure.
//
// The three self-identity lookups below (os.Executable, os.Stat, f.Stat) are
// different: resolved is already open at this point (the fd-based exec is
// already safe to proceed with), and these calls exist only to run the
// redundant self-binary re-check this hardening adds on top of the caller's
// own isSelfBinary check. If one of them errors — os.Executable() failing in
// an unusual runtime environment, say — that's a reason to skip the redundant
// check, not a reason to throw away the fd we already have and fall back to
// an unpinned path-based exec. So only a confirmed self-binary match aborts;
// any other failure here just proceeds with the fd-based exec un-re-checked,
// still strictly safer than falling back to exec-by-path.
func execViaFD(execCmd *exec.Cmd, resolved string) (*os.File, error) {
	fd, err := syscall.Open(resolved, oPath, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot open %q for fd-based exec: %w", resolved, err)
	}
	f := os.NewFile(uintptr(fd), resolved)

	if self, err := os.Executable(); err == nil {
		if selfInfo, err := os.Stat(self); err == nil {
			if fdInfo, err := f.Stat(); err == nil {
				if os.SameFile(selfInfo, fdInfo) {
					f.Close()
					return nil, &SecurityError{Command: resolved, Reason: "command resolves to the running rakitsu binary"}
				}
			}
		}
	}

	execCmd.ExtraFiles = []*os.File{f}
	execCmd.Path = "/proc/self/fd/3"
	return f, nil
}
