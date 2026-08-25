package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestHandleBrowse_RejectsSiblingDirectoryWithSamePrefix regression-guards:
// the home/cwd gate used strings.HasPrefix(absPath, homeDir) with no
// separator, so a sibling directory that merely starts with the same string
// (e.g. homeDir "/home/alice", target "/home/alice-evil") passed the check
// even though it isn't actually under homeDir. filepath.Abs cleans literal
// "../" traversal, but this sibling-directory bypass isn't caught by that.
func TestHandleBrowse_RejectsSiblingDirectoryWithSamePrefix(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "alice")
	sibling := filepath.Join(root, "alice-evil")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sibling, "leaked-dir"), 0755); err != nil {
		t.Fatalf("mkdir sibling: %v", err)
	}

	t.Setenv("HOME", home) // os.UserHomeDir() reads $HOME on unix
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(origWd)
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir home: %v", err)
	}

	s := &SSEServer{}
	r := httptest.NewRequest("GET", "/api/browse?path="+sibling, nil)
	w := httptest.NewRecorder()
	s.handleBrowse(w, r)

	var resp struct {
		Path  string   `json:"path"`
		Dirs  []string `json:"dirs"`
		Error string   `json:"error,omitempty"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == "" {
		t.Errorf("browsing %q (a sibling of homeDir %q sharing only a string prefix) should be rejected, got dirs=%v", sibling, home, resp.Dirs)
	}
}

// TestHandleBrowse_AllowsGenuineSubdirectoryOfHome guards against
// overcorrecting the fix above: a real subdirectory of homeDir must still be
// browsable.
func TestHandleBrowse_AllowsGenuineSubdirectoryOfHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "alice")
	sub := filepath.Join(home, "projects")
	if err := os.MkdirAll(filepath.Join(sub, "inner"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Setenv("HOME", home)
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(origWd)
	if err := os.Chdir(home); err != nil {
		t.Fatalf("chdir home: %v", err)
	}

	s := &SSEServer{}
	r := httptest.NewRequest("GET", "/api/browse?path="+sub, nil)
	w := httptest.NewRecorder()
	s.handleBrowse(w, r)

	var resp struct {
		Path  string   `json:"path"`
		Dirs  []string `json:"dirs"`
		Error string   `json:"error,omitempty"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("browsing a genuine subdirectory of home should be allowed, got error=%q", resp.Error)
	}
	if len(resp.Dirs) != 1 || resp.Dirs[0] != "inner" {
		t.Errorf("dirs = %v, want [inner]", resp.Dirs)
	}
}
