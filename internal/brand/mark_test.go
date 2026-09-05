package brand

import (
	"strings"
	"testing"
)

// TestMark_Dimensions catches an edit to Mark that leaves a line the wrong
// length — every consumer assumes a uniform MarkWidth x MarkHeight grid.
func TestMark_Dimensions(t *testing.T) {
	if len(Mark) != MarkHeight {
		t.Fatalf("len(Mark) = %d, want MarkHeight = %d", len(Mark), MarkHeight)
	}
	for i, line := range Mark {
		if n := len([]rune(line)); n != MarkWidth {
			t.Errorf("Mark[%d] has %d runes, want MarkWidth = %d: %q", i, n, MarkWidth, line)
		}
	}
}

// TestRender_ContainsMark checks Render doesn't just return styling escape
// codes with the actual glyph lost.
func TestRender_ContainsMark(t *testing.T) {
	got := Render()
	for i, line := range Mark {
		if !strings.Contains(got, line) {
			t.Errorf("Render() missing Mark[%d] verbatim: %q", i, line)
		}
	}
}
