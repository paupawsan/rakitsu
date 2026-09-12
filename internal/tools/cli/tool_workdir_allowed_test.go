package cli

import (
	"os"
	"testing"
)

// Regression for the --workdir / allowed_paths mismatch on the cli side:
// checkArgPaths resolves argument tokens relative to dir (the workdir) but
// resolved relative allowed_paths entries against the process cwd, so with
// allowed_paths: ["."] and a workdir elsewhere even `sh -c ls` was blocked
// ("path '-c' is not in allowed paths").
func TestCheckArgPaths_RelativeAllowedResolvesAgainstDir(t *testing.T) {
	workdir := t.TempDir()
	elsewhere := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	if err := checkArgPaths([]string{"sh", "-c", "ls -la"}, workdir, []string{"."}); err != nil {
		t.Fatalf("sh -c inside the workdir must pass with allowed_paths [\".\"], got: %v", err)
	}

	// A token naming the launch cwd (outside the workdir) must still be denied.
	if err := checkArgPaths([]string{"cat", elsewhere + "/secret"}, workdir, []string{"."}); err == nil {
		t.Fatal("path outside the workdir must be denied")
	}
}
