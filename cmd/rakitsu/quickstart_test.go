package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestServeCmd_DefinesRun guards the quickstart → serve hand-off.
// runQuickstart starts the web UI by calling serveCmd.Run directly. If
// serveCmd is ever switched to RunE-only, serveCmd.Run becomes nil and
// `rakitsu quickstart` segfaults on its final step.
func TestServeCmd_DefinesRun(t *testing.T) {
	if serveCmd.Run == nil {
		t.Fatal("serveCmd.Run is nil — runQuickstart invokes serveCmd.Run directly; " +
			"serve must keep a Run handler (not RunE-only) or quickstart will panic")
	}
}

// Regression: re-running quickstart into an already-scaffolded directory
// used to overwrite existing files with no warning. existingQuickstartFiles
// is the pure detection logic behind the confirm-before-overwrite prompt.

func TestExistingQuickstartFiles_ModularDetectsConflict(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"config.yaml": "new", "agents/assistant.md": "new"}

	got := existingQuickstartFiles(files, dir, true)
	if len(got) != 1 || got[0] != filepath.Join(dir, "config.yaml") {
		t.Errorf("existingQuickstartFiles = %v, want just config.yaml flagged", got)
	}
}

func TestExistingQuickstartFiles_ModularNoConflictOnFreshDir(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{"config.yaml": "new", "agents/assistant.md": "new"}

	got := existingQuickstartFiles(files, dir, true)
	if len(got) != 0 {
		t.Errorf("existingQuickstartFiles = %v, want none on a fresh directory", got)
	}
}

func TestExistingQuickstartFiles_SingleFileDetectsConflict(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	got := existingQuickstartFiles(map[string]string{"llm-chat.yaml": "new"}, dir, false)
	if len(got) != 1 || got[0] != filepath.Join(dir, "config.yaml") {
		t.Errorf("existingQuickstartFiles = %v, want config.yaml flagged", got)
	}
}
