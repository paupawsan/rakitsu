package chat

import (
	"strings"
	"testing"
)

// TestClampToHeight_Shorter pads output when it has fewer lines than the
// target height. Without this, alt-screen renderers can leave stale frames
// from previous View() calls visible.
func TestClampToHeight_Shorter(t *testing.T) {
	in := "a\nb\nc"
	out := clampToHeight(in, 5)
	got := strings.Count(out, "\n") + 1
	if got != 5 {
		t.Errorf("3-line input clamped to height=5 should produce 5 lines, got %d: %q", got, out)
	}
	if !strings.HasPrefix(out, "a\nb\nc\n") {
		t.Errorf("original content should be preserved at top: %q", out)
	}
}

// TestClampToHeight_Longer truncates output when it has more lines than the
// target height. Without this, excess lines can push the status bar
// off-screen or bleed into the scrollback.
func TestClampToHeight_Longer(t *testing.T) {
	in := "a\nb\nc\nd\ne\nf\ng"
	out := clampToHeight(in, 3)
	got := strings.Count(out, "\n") + 1
	if got != 3 {
		t.Errorf("7-line input clamped to height=3 should produce 3 lines, got %d: %q", got, out)
	}
	if out != "a\nb\nc" {
		t.Errorf("expected first 3 lines, got %q", out)
	}
}

// TestClampToHeight_Exact is a no-op when the input already matches height.
func TestClampToHeight_Exact(t *testing.T) {
	in := "a\nb\nc"
	out := clampToHeight(in, 3)
	if out != in {
		t.Errorf("exact-height input should be unchanged, got %q", out)
	}
}

// TestClampToHeight_ZeroHeight is a safety guard — if height is non-positive
// (e.g. before WindowSizeMsg arrives), return input unchanged rather than
// truncating to zero.
func TestClampToHeight_ZeroHeight(t *testing.T) {
	in := "a\nb\nc"
	if out := clampToHeight(in, 0); out != in {
		t.Errorf("height=0 should return input unchanged, got %q", out)
	}
	if out := clampToHeight(in, -5); out != in {
		t.Errorf("negative height should return input unchanged, got %q", out)
	}
}

// TestClampToHeight_Empty handles the boundary where there's no content yet.
func TestClampToHeight_Empty(t *testing.T) {
	out := clampToHeight("", 4)
	got := strings.Count(out, "\n") + 1
	if got != 4 {
		t.Errorf("empty input clamped to height=4 should produce 4 lines, got %d: %q", got, out)
	}
}
