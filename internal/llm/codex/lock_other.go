//go:build !unix && !windows

package codex

// lockFile is a no-op on platforms with neither flock nor LockFileEx;
// in-process exclusion via tokenSource.mu still holds, only cross-process
// refreshes are unguarded there.
func lockFile(path string) (func(), error) {
	return func() {}, nil
}
