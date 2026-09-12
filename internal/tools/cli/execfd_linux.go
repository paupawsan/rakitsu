//go:build linux

package cli

import (
	"fmt"
	"os"
	"os/exec"
)

// execViaFD re-execs the already-opened, already-verified file f through
// its live file descriptor (via the Linux-only /proc/self/fd magic
// symlink) instead of re-specifying its path — eliminating the
// check-then-exec race entirely on this platform. execve resolving
// /proc/self/fd/N loads exactly the inode f refers to, regardless of
// anything that happens to the original path afterward; this is the same
// technique tools like sudo and runc use for equivalent problems.
//
// f is passed via execCmd.ExtraFiles, which os/exec guarantees lands at
// fd 3 in the child (0/1/2 are stdin/stdout/stderr) — this is the only
// entry, so it is always fd 3. execCmd.Path is overridden to the /proc
// path; execCmd.Args[0] (already set to the resolved path by the caller)
// is left as-is purely for cosmetic/self-identification purposes inside
// the child (argv[0] need not match the path actually exec'd).
func execViaFD(execCmd *exec.Cmd, f *os.File) {
	execCmd.ExtraFiles = []*os.File{f}
	execCmd.Path = fmt.Sprintf("/proc/self/fd/%d", 3)
}
