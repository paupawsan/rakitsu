package chat

import (
	"strings"
	"testing"
)

// TestRenderBanner_ContainsIdentity checks the banner surfaces the identity
// info it's for (version/agent/model) rather than just drawing a box.
func TestRenderBanner_ContainsIdentity(t *testing.T) {
	got := renderBanner(80, "v0.2.0-alpha.3.318", "ChatHost", "chat-host (gpt-5.6-luna)")

	for _, want := range []string{"v0.2.0-alpha.3.318", "ChatHost", "chat-host (gpt-5.6-luna)"} {
		if !strings.Contains(got, want) {
			t.Errorf("banner missing %q:\n%s", want, got)
		}
	}
}

// TestRenderBanner_TooNarrow returns "" rather than a garbled box when the
// terminal is too small to draw one sensibly.
func TestRenderBanner_TooNarrow(t *testing.T) {
	if got := renderBanner(10, "v1.0.0", "Agent", "gpt-5"); got != "" {
		t.Errorf("expected empty banner for width=10, got %q", got)
	}
}

// TestRenderBanner_EmptyVersion still renders (falls back to a bare
// "rakitsu" version line) rather than leaving a blank hole in the box.
func TestRenderBanner_EmptyVersion(t *testing.T) {
	got := renderBanner(80, "", "Agent", "gpt-5")
	if !strings.Contains(got, "rakitsu") {
		t.Errorf("expected fallback version line, got %q", got)
	}
}
