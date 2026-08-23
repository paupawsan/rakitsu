package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// ============================================================
// ThinkingPlaceholder
// ============================================================

func TestThinkingPlaceholder_Short(t *testing.T) {
	content := "decided to use write_file"
	got := ThinkingPlaceholder(0, content)
	want := "[Thought 1: decided to use write_file]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestThinkingPlaceholder_Truncated(t *testing.T) {
	content := strings.Repeat("x", 400)
	got := ThinkingPlaceholder(2, content)
	if !strings.HasPrefix(got, "[Thought 3: ") {
		t.Errorf("wrong prefix: %q", got)
	}
	if !strings.HasSuffix(got, "...]") {
		t.Errorf("expected ellipsis suffix: %q", got)
	}
	// body should be exactly thinkingPlaceholderMax chars
	inner := strings.TrimPrefix(got, "[Thought 3: ")
	inner = strings.TrimSuffix(inner, "...]")
	if len(inner) != thinkingPlaceholderMax {
		t.Errorf("truncated body length = %d, want %d", len(inner), thinkingPlaceholderMax)
	}
}

func TestThinkingPlaceholder_ExactBoundary(t *testing.T) {
	content := strings.Repeat("a", thinkingPlaceholderMax)
	got := ThinkingPlaceholder(0, content)
	// Exactly at limit — no ellipsis
	if strings.HasSuffix(got, "...]") {
		t.Errorf("should not have ellipsis at exact boundary: %q", got)
	}
}

// TestThinkingPlaceholder_TruncatesOnRuneBoundary regression-guards against
// byte-offset truncation splitting a multi-byte UTF-8 character mid-codepoint.
func TestThinkingPlaceholder_TruncatesOnRuneBoundary(t *testing.T) {
	content := strings.Repeat("思", thinkingPlaceholderMax) // 3 bytes/rune; well over the byte cap
	got := ThinkingPlaceholder(0, content)
	if !utf8.ValidString(got) {
		t.Errorf("ThinkingPlaceholder produced invalid UTF-8: %q", got)
	}
}

// ============================================================
// ThinkingStore
// ============================================================

func TestThinkingStore_Save(t *testing.T) {
	tmp := t.TempDir()
	// Patch home-dir by using a manually created store (dir is exported-ish via the constructor)
	// To avoid depending on os.UserHomeDir, test the internals via a temp-dir store.
	store := &ThinkingStore{dir: tmp}

	if err := store.Save("my-agent", 3, "I thought about X"); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	expected := filepath.Join(tmp, "my-agent-3.md")
	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("file not created at %s: %v", expected, err)
	}
	if string(data) != "I thought about X" {
		t.Errorf("content mismatch: got %q", string(data))
	}
}

func TestThinkingStore_NilReceiver(t *testing.T) {
	var ts *ThinkingStore
	// Must not panic
	if err := ts.Save("agent", 0, "thinking"); err != nil {
		t.Errorf("nil ThinkingStore.Save should be no-op, got: %v", err)
	}
}

func TestThinkingStore_SanitizesFileName(t *testing.T) {
	tmp := t.TempDir()
	store := &ThinkingStore{dir: tmp}
	_ = store.Save("my:agent/name", 0, "content")
	// File must exist with invalid chars replaced
	expected := filepath.Join(tmp, "my_agent_name-0.md")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("expected sanitized file %s to exist: %v", expected, err)
	}
}

// TestNewThinkingStore_SanitizesSessionID regression-guards against
// NewThinkingStore using a raw, unsanitized sessionID as a path component —
// inconsistent with Save's sanitizeFileName treatment of agentName.
func TestNewThinkingStore_SanitizesSessionID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	store := NewThinkingStore("weird:session/id")
	if store == nil {
		t.Fatal("NewThinkingStore returned nil")
	}
	if strings.ContainsAny(store.dir, ":/\\") && !strings.HasPrefix(store.dir, tmp) {
		t.Errorf("store.dir = %q still contains unsanitized separators from sessionID", store.dir)
	}
	// The directory actually created on disk must match the sanitized name.
	expected := filepath.Join(tmp, ".rakitsu", "thinking", "weird_session_id")
	if store.dir != expected {
		t.Errorf("store.dir = %q, want %q", store.dir, expected)
	}
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("expected sanitized session dir %s to exist: %v", expected, err)
	}
}

// ============================================================
// extractOpenAIThinking (tested via public-facing behaviour via openai package;
// replicate logic here to keep test self-contained)
// ============================================================

// localExtract replicates the openai package's extractOpenAIThinking for testing here.
// The real function lives in internal/llm/openai — tested separately via that package's tests.
// Here we verify ThinkingStore + ThinkingPlaceholder compose correctly.

func TestThinkingStore_MultipleIterations(t *testing.T) {
	tmp := t.TempDir()
	store := &ThinkingStore{dir: tmp}

	for i := 0; i < 3; i++ {
		if err := store.Save("agent", i, "thought content"); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}
	entries, _ := os.ReadDir(tmp)
	if len(entries) != 3 {
		t.Errorf("expected 3 files, got %d", len(entries))
	}
}
