package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withInteractiveFlag(t *testing.T, v bool) {
	t.Helper()
	prev := interactiveFlag
	interactiveFlag = v
	t.Cleanup(func() { interactiveFlag = prev })
}

func TestRunArgs_ZeroArgsRequiresInteractive(t *testing.T) {
	withInteractiveFlag(t, true)
	if err := runCmd.Args(runCmd, nil); err != nil {
		t.Errorf("zero args with --interactive: got error %v, want nil", err)
	}
}

func TestRunArgs_ZeroArgsWithoutInteractiveStillFails(t *testing.T) {
	withInteractiveFlag(t, false)
	if err := runCmd.Args(runCmd, nil); err == nil {
		t.Error("zero args without --interactive: got nil error, want an error")
	}
}

func TestRunArgs_ExistingMultiArgBehaviorUnaffected(t *testing.T) {
	for _, interactive := range []bool{true, false} {
		withInteractiveFlag(t, interactive)
		if err := runCmd.Args(runCmd, []string{"agent.yaml"}); err != nil {
			t.Errorf("interactive=%v, 1 arg: got error %v, want nil", interactive, err)
		}
		if err := runCmd.Args(runCmd, []string{"agent.yaml", "query"}); err != nil {
			t.Errorf("interactive=%v, 2 args: got error %v, want nil", interactive, err)
		}
	}
}

func TestEnsureDefaultConfig_CreatesOnceAndPreservesEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "default-agent.yaml")

	if err := ensureDefaultConfig(path); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist after first call: %v", err)
	}

	marker := "\n# user edit marker\n"
	if err := appendToFile(path, marker); err != nil {
		t.Fatalf("appending marker: %v", err)
	}

	if err := ensureDefaultConfig(path); err != nil {
		t.Fatalf("second call: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if !strings.Contains(string(content), "user edit marker") {
		t.Error("second call overwrote the file — user edit was lost")
	}
}

func TestDefaultConfigPath_UnderHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	got, err := defaultConfigPath()
	if err != nil {
		t.Fatalf("defaultConfigPath: %v", err)
	}
	want := filepath.Join(dir, ".rakitsu", "default-agent.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}
