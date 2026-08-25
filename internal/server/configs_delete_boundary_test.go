package server

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfigStore_Delete_RejectsSiblingDirectoryWithSamePrefix regression-
// guards: Delete gated on strings.HasPrefix(abs, tempAbs) with no separator,
// so a sibling directory that merely starts with the same string (e.g.
// tempDir ".../cfgstore", target ".../cfgstore-evil") satisfied the check
// even though it isn't actually under tempDir. Same shape as the handleBrowse
// bug fixed elsewhere in this package (see sse.go's withinDir). Low real-
// world impact today — cs.entries only ever holds paths this store itself
// wrote — but worth closing for defense-in-depth and consistency.
func TestConfigStore_Delete_RejectsSiblingDirectoryWithSamePrefix(t *testing.T) {
	cs, err := NewConfigStore(nil)
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}

	root := t.TempDir()
	tempDir := filepath.Join(root, "cfgstore")
	sibling := filepath.Join(root, "cfgstore-evil")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("mkdir tempDir: %v", err)
	}
	if err := os.MkdirAll(sibling, 0755); err != nil {
		t.Fatalf("mkdir sibling: %v", err)
	}
	cs.tempDir = tempDir

	evilFile := filepath.Join(sibling, "leaked.yaml")
	if err := os.WriteFile(evilFile, []byte("name: evil\n"), 0644); err != nil {
		t.Fatalf("write evil file: %v", err)
	}
	cs.entries["evil-id"] = evilFile

	if err := cs.Delete("evil-id"); err == nil {
		t.Error("Delete on a file in a sibling dir sharing tempDir's string prefix should be rejected, but it succeeded")
	}
	if _, err := os.Stat(evilFile); err != nil {
		t.Errorf("file should not have been removed, but stat failed: %v", err)
	}
}
