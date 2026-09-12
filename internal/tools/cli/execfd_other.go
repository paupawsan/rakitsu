//go:build !linux

package cli

import (
	"os"
	"os/exec"
)

// execViaFD is a no-op on non-Linux platforms: there is no portable
// equivalent to Linux's /proc/self/fd magic symlink (macOS/BSD have no
// procfs, and Go's stdlib exposes no portable fexecve), so exec proceeds
// via execCmd's already-set Path (the resolved absolute path) as before.
// The self-invocation check the caller already ran (isSelfBinary on the
// resolved path) still applies — it just can't be pinned all the way
// through to the actual exec syscall on these platforms. See
// docs/SECURITY.md.
func execViaFD(execCmd *exec.Cmd, resolved string) (*os.File, error) {
	return nil, nil
}
