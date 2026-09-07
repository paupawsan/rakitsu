package chat

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
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

// assertViewFitsWidth is shared by TestView_LongModelNameFitsWidth and
// TestView_LongNoticeFitsWidth: every rendered line must fit m.width (a
// line the terminal has to wrap counts as 1 line to clampToHeight but 2
// rows on screen, desyncing bubbletea's alt-screen redraw), and the total
// line count must still be exactly m.height.
func assertViewFitsWidth(t *testing.T, m Model) {
	t.Helper()
	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Fatalf("View() produced %d lines, want exactly %d", len(lines), m.height)
	}
	// The status bar is the LAST line View() composes, so it's the first
	// thing clampToHeight's truncation drops if the pre-clamp total runs
	// long — which silently makes the width check below a no-op instead of
	// a failure. " tokens:" is always present in statusBar()'s output, so
	// its absence here means this test stopped checking what it claims to.
	if !strings.Contains(out, " tokens:") {
		t.Fatalf("status bar (\" tokens: ...\") not found anywhere in View() output — " +
			"it was likely truncated away by clampToHeight; check the model's chrome " +
			"setup (e.g. textarea height) against handleResize()'s assumptions")
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > m.width {
			t.Errorf("line %d width = %d, want <= %d (m.width): %q", i, w, m.width, l)
		}
	}
}

// TestView_LongModelNameFitsWidth is a regression for the chat TUI screen
// corruption: the title bar (agentName + modelName) had no width clamp,
// unlike the sticky header and input-row prefix. A realistic long LiteLLM
// model alias overflowed it (86 cols rendered in an 80-col terminal).
func TestView_LongModelNameFitsWidth(t *testing.T) {
	m := Model{width: 80, height: 24, historyIdx: -1, spinner: spinner.New()}
	m.textarea = textarea.New()
	m.textarea.SetHeight(3) // matches NewModel's real chrome setup
	m = m.handleResize()
	m.agentName = "TechLead"
	m.modelName = "vllm-nemotron-elastic-30b-long-context-window-experimental"
	m.blocks = []ContentBlock{{Type: BlockUser, Text: "hi"}}
	m.updateViewport()

	assertViewFitsWidth(t, m)
}

// TestView_LongNoticeFitsWidth is a regression for the same class of bug in
// statusBar(): a long notice (e.g. after a copy) plus a large token count
// could push the status line past m.width with no truncation.
func TestView_LongNoticeFitsWidth(t *testing.T) {
	m := Model{width: 55, height: 24, historyIdx: -1, spinner: spinner.New()}
	m.textarea = textarea.New()
	m.textarea.SetHeight(3) // matches NewModel's real chrome setup
	m = m.handleResize()
	m.totalTokens = 123456789
	m.notice = "copied reply 3/12 to clipboard"
	m.blocks = []ContentBlock{{Type: BlockUser, Text: "hi"}}
	m.updateViewport()

	assertViewFitsWidth(t, m)
}

// TestView_HugeTokenCountFitsWidth is a regression for a gap the previous
// two tests didn't cover: statusBar() clamped rightRaw (the notice/hint/
// spinner text) to whatever room was left after leftRaw's width, but never
// clamped leftRaw (" tokens: <N>") itself. On a narrow terminal with a
// large enough totalTokens — plausible over a long real session, since
// it's an unbounded running count — leftRaw alone can exceed m.width,
// which zeroes the computed gap but leaves left+right wider than m.width:
// the same physical-wrap-desyncs-the-alt-screen failure this whole file
// exists to catch, just from the left side of the status bar.
func TestView_HugeTokenCountFitsWidth(t *testing.T) {
	m := Model{width: 20, height: 24, historyIdx: -1, spinner: spinner.New()}
	m.textarea = textarea.New()
	m.textarea.SetHeight(3) // matches NewModel's real chrome setup
	m = m.handleResize()
	m.totalTokens = 999999999999
	m.blocks = []ContentBlock{{Type: BlockUser, Text: "hi"}}
	m.updateViewport()

	assertViewFitsWidth(t, m)
}

// TestView_NarrowWidthStatusBarNoDoubleFloor is a regression for a second,
// independent overflow in statusBar(): leftMax and avail each used to floor
// to their own minimum of 4, so on a terminal narrow enough that m.width -
// 4 was itself below 4 (i.e. 4 <= m.width < 8), both floors could trigger
// at once and claim 4+4=8 columns combined — wider than m.width, the exact
// failure class this file exists to catch, just needing a narrower
// terminal than TestView_HugeTokenCountFitsWidth's width=20 to reach.
// Sweeps every width from 0 to 12 (comfortably past the 4-8 danger zone,
// including the pre-resize width=0 case) rather than picking one value,
// since the bug was specific to a narrow band other single-width tests in
// this file don't happen to cross.
func TestView_NarrowWidthStatusBarNoDoubleFloor(t *testing.T) {
	for w := 0; w <= 12; w++ {
		m := Model{width: w, height: 24, historyIdx: -1, spinner: spinner.New()}
		m.textarea = textarea.New()
		m.textarea.SetHeight(3) // matches NewModel's real chrome setup
		m = m.handleResize()
		m.totalTokens = 999999999999
		m.blocks = []ContentBlock{{Type: BlockUser, Text: "hi"}}
		m.updateViewport()

		out := m.statusBar()
		if got := lipgloss.Width(out); got > w {
			t.Errorf("width=%d: statusBar() width = %d, want <= %d: %q", w, got, w, out)
		}
	}
}
