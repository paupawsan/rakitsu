package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression: rakitsu scaffold used to overwrite an existing output file/
// directory with no warning. writeFile/writeDir must refuse unless force
// is set, and must not partially write a modular layout before detecting
// a conflict.

func TestWriteFile_RefusesExistingWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "llm-chat.yaml")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := writeFile(path, map[string]string{"llm-chat.yaml": "new"}, false)
	if err == nil {
		t.Fatal("expected an error when the output file already exists and force=false")
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "original" {
		t.Errorf("file content = %q, want untouched %q", got, "original")
	}
}

func TestWriteFile_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "llm-chat.yaml")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeFile(path, map[string]string{"llm-chat.yaml": "new"}, true); err != nil {
		t.Fatalf("writeFile with force=true: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("file content = %q, want %q", got, "new")
	}
}

func TestWriteDir_RefusesExistingWithoutForce_NoPartialWrite(t *testing.T) {
	dir := t.TempDir()
	// Only one of the two target files already exists.
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	agentPath := filepath.Join(dir, "agents", "assistant.md")

	files := map[string]string{
		"config.yaml":         "new-config",
		"agents/assistant.md": "new-agent",
	}
	err := writeDir(dir, files, false)
	if err == nil {
		t.Fatal("expected an error when a target file already exists and force=false")
	}

	if _, statErr := os.Stat(agentPath); statErr == nil {
		t.Error("writeDir wrote agents/assistant.md despite refusing the conflicting config.yaml — conflict check must run before any write")
	}
	got, readErr := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "original" {
		t.Errorf("config.yaml = %q, want untouched %q", got, "original")
	}
}

func TestWriteDir_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{"config.yaml": "new-config"}
	if err := writeDir(dir, files, true); err != nil {
		t.Fatalf("writeDir with force=true: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-config" {
		t.Errorf("config.yaml = %q, want %q", got, "new-config")
	}
}
