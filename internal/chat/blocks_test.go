package chat

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestContentBlock_RenderUser(t *testing.T) {
	b := ContentBlock{Type: BlockUser, Text: "hello"}
	r := b.Render()
	if !strings.Contains(r, "You:") || !strings.Contains(r, "hello") {
		t.Errorf("user block render = %q, expected to contain 'You:' and 'hello'", r)
	}
	// Branch chip must NOT appear when BranchCount is zero — single-branch
	// turns get the plain "**You:**" header, no decoration.
	if strings.Contains(r, "‹") {
		t.Errorf("chip rendered on single-branch user block: %q", r)
	}
}

func TestContentBlock_RenderUserBranchChip(t *testing.T) {
	// Phase 2 PR E-1 — user blocks with BranchCount > 1 carry an inline
	// `‹n/m›` chip so the user can see other branches exist for this
	// turn. The chip rides the **You:** header line so the blockquote
	// stays clean. BranchIndex is 0-indexed; the rendered chip uses
	// 1-indexed numerator to match human counting.
	b := ContentBlock{Type: BlockUser, Text: "what is 2+2?", BranchIndex: 1, BranchCount: 3}
	r := b.Render()
	if !strings.Contains(r, "‹2/3›") {
		t.Errorf("BranchCount>1 should yield ‹n/m› chip, got: %q", r)
	}
	// Chip should be on the same line as the **You:** header so the
	// blockquote body underneath is untouched.
	if !strings.Contains(r, "**You:** `‹2/3›`") {
		t.Errorf("chip not on the You: header line, got: %q", r)
	}
	// And the user text still renders as a blockquote.
	if !strings.Contains(r, "> what is 2+2?") {
		t.Errorf("user text blockquote lost when chip is present: %q", r)
	}
}

func TestContentBlock_RenderUserSingleBranchNoChip(t *testing.T) {
	// Single-branch turn (count=1) means there's nothing to switch to —
	// chip must stay hidden to avoid implying choice where there isn't one.
	b := ContentBlock{Type: BlockUser, Text: "hi", BranchIndex: 0, BranchCount: 1}
	r := b.Render()
	if strings.Contains(r, "‹") {
		t.Errorf("single-branch turn rendered a chip: %q", r)
	}
}

func TestContentBlock_RenderUserMultiLine(t *testing.T) {
	b := ContentBlock{Type: BlockUser, Text: "line1\nline2\nline3"}
	r := b.Render()
	// Each line should be prefixed with "> " (blockquote)
	for _, want := range []string{"> line1", "> line2", "> line3"} {
		if !strings.Contains(r, want) {
			t.Errorf("multi-line user block missing %q in: %q", want, r)
		}
	}
}

func TestContentBlock_RenderAssistantEmpty(t *testing.T) {
	b := ContentBlock{Type: BlockAssistant, Text: ""}
	if r := b.Render(); r != "" {
		t.Errorf("empty assistant block should render empty, got %q", r)
	}
}

func TestContentBlock_RenderAssistant(t *testing.T) {
	b := ContentBlock{Type: BlockAssistant, Text: "hello world"}
	r := b.Render()
	if !strings.Contains(r, "hello world") {
		t.Errorf("assistant block render = %q, expected 'hello world'", r)
	}
}

func TestContentBlock_RenderToolRunning(t *testing.T) {
	b := ContentBlock{Type: BlockTool, ToolName: "search", ToolArgs: `{"q":"test"}`, ToolDone: false}
	r := b.Render()
	if !strings.Contains(r, "search") {
		t.Errorf("tool block should contain tool name, got %q", r)
	}
	if !strings.Contains(r, "⏳") {
		t.Errorf("running tool should show spinner, got %q", r)
	}
}

func TestContentBlock_RenderToolDone(t *testing.T) {
	b := ContentBlock{Type: BlockTool, ToolName: "search", ToolDone: true, Duration: 150, Text: "result"}
	r := b.Render()
	if !strings.Contains(r, "✓") {
		t.Errorf("done tool should show check, got %q", r)
	}
	if !strings.Contains(r, "150ms") {
		t.Errorf("done tool should show duration, got %q", r)
	}
}

func TestContentBlock_RenderToolError(t *testing.T) {
	b := ContentBlock{Type: BlockTool, ToolName: "search", ToolDone: true, ToolErr: "failed"}
	r := b.Render()
	if !strings.Contains(r, "✗") {
		t.Errorf("error tool should show X, got %q", r)
	}
	if !strings.Contains(r, "failed") {
		t.Errorf("error tool should show error message, got %q", r)
	}
}

func TestContentBlock_RenderToolLongArgs(t *testing.T) {
	longArgs := strings.Repeat("x", 100)
	b := ContentBlock{Type: BlockTool, ToolName: "search", ToolArgs: longArgs, ToolDone: false}
	r := b.Render()
	if len(b.ToolArgs) > 80 && !strings.Contains(r, "...") {
		t.Errorf("long args should be truncated with '...', got %q", r)
	}
}

// TestContentBlock_RenderToolLongArgsNonASCII regression-guards: the
// truncation was a byte-offset slice (args[:77]), not a rune-offset one —
// any tool argument with non-ASCII text (a file path, a search string) past
// byte 77 could get sliced mid-rune, producing invalid UTF-8 in the
// rendered block. previewLine (context_snapshot.go) already does this
// correctly for the same purpose.
func TestContentBlock_RenderToolLongArgsNonASCII(t *testing.T) {
	// 90 multi-byte runes — well past the 77-byte truncation point, so a
	// byte-offset slice lands mid-rune (each "é" is 2 bytes).
	longArgs := strings.Repeat("é", 90)
	b := ContentBlock{Type: BlockTool, ToolName: "search", ToolArgs: longArgs, ToolDone: false}
	r := b.Render()
	if !utf8.ValidString(r) {
		t.Fatalf("rendered block is not valid UTF-8 — truncation sliced mid-rune: %q", r)
	}
}

func TestContentBlock_RenderToolOutputTruncated(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "line"
	}
	b := ContentBlock{Type: BlockTool, ToolName: "search", ToolDone: true, Text: strings.Join(lines, "\n"), Duration: 100}
	r := b.Render()
	if !strings.Contains(r, "more lines") {
		t.Errorf("long output should be truncated, got %q", r)
	}
}

func TestContentBlock_RenderSystem(t *testing.T) {
	b := ContentBlock{Type: BlockSystem, Text: "info"}
	r := b.Render()
	if !strings.Contains(r, "info") {
		t.Errorf("system block should contain text, got %q", r)
	}
}

// TestContentBlock_RenderToolNested verifies a sub-agent tool call
// renders with deeper blockquote indentation and the agent name in the
// header, so it reads as nested rather than flat.
func TestContentBlock_RenderToolNested(t *testing.T) {
	root := ContentBlock{Type: BlockTool, ToolName: "search", Depth: 0}
	sub := ContentBlock{Type: BlockTool, ToolName: "search", AgentName: "Researcher", Depth: 1}

	rootR, subR := root.Render(), sub.Render()
	if !strings.Contains(subR, "Researcher → search") {
		t.Errorf("nested tool should name the sub-agent, got %q", subR)
	}
	if strings.Contains(rootR, "→") {
		t.Errorf("root tool should not show an agent arrow, got %q", rootR)
	}
	// Depth 1 nests an extra blockquote level ("> > " vs "> ").
	if !strings.Contains(subR, "> > ") {
		t.Errorf("depth-1 tool should use a nested blockquote, got %q", subR)
	}
}

// TestContentBlock_RenderReasoning verifies streaming CoT renders as a
// dimmed "thinking" blockquote containing the reasoning text.
func TestContentBlock_RenderReasoning(t *testing.T) {
	b := ContentBlock{Type: BlockReasoning, Text: "first I consider the files"}
	r := b.Render()
	if !strings.Contains(r, "thinking") {
		t.Errorf("reasoning block should be labelled 'thinking', got %q", r)
	}
	if !strings.Contains(r, "first I consider the files") {
		t.Errorf("reasoning block should contain the CoT text, got %q", r)
	}
}

// TestContentBlock_RenderReasoningEmpty — an empty reasoning block renders
// nothing (no stray "thinking" label before any CoT has streamed).
func TestContentBlock_RenderReasoningEmpty(t *testing.T) {
	b := ContentBlock{Type: BlockReasoning, Text: ""}
	if r := b.Render(); r != "" {
		t.Errorf("empty reasoning block should render empty, got %q", r)
	}
}

// TestContentBlock_RenderReasoningTailTruncated — long reasoning keeps the
// most recent lines (the useful progress signal) and notes the elision.
func TestContentBlock_RenderReasoningTailTruncated(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "step"
	}
	lines[29] = "LAST-LINE"
	b := ContentBlock{Type: BlockReasoning, Text: strings.Join(lines, "\n")}
	r := b.Render()
	if !strings.Contains(r, "earlier lines") {
		t.Errorf("long reasoning should note truncated lines, got %q", r)
	}
	if !strings.Contains(r, "LAST-LINE") {
		t.Errorf("reasoning truncation should keep the most recent line, got %q", r)
	}
}

// TestContentBlock_RenderReasoningCollapsed — a collapsed reasoning block is
// a one-line summary: the ▸ glyph, a line count, no body text.
func TestContentBlock_RenderReasoningCollapsed(t *testing.T) {
	b := ContentBlock{Type: BlockReasoning, Text: "aaa\nbbb\nccc", Collapsed: true}
	r := b.Render()
	if !strings.Contains(r, "▸") {
		t.Errorf("collapsed reasoning should show the ▸ glyph, got %q", r)
	}
	if !strings.Contains(r, "3 lines") {
		t.Errorf("collapsed reasoning should summarize the line count, got %q", r)
	}
	if strings.Contains(r, "bbb") {
		t.Errorf("collapsed reasoning must hide its body, got %q", r)
	}
}

// TestContentBlock_RenderToolCollapsed — a collapsed completed tool shows the
// header (name, status) with a ▸ glyph and a line count, but not the output.
func TestContentBlock_RenderToolCollapsed(t *testing.T) {
	b := ContentBlock{
		Type: BlockTool, ToolName: "search", ToolDone: true, Duration: 12,
		Text: "r1\nr2\nr3\nr4", Collapsed: true,
	}
	r := b.Render()
	if !strings.Contains(r, "▸") || !strings.Contains(r, "search") {
		t.Errorf("collapsed tool should show ▸ glyph + name, got %q", r)
	}
	if !strings.Contains(r, "4 lines") {
		t.Errorf("collapsed tool should summarize output line count, got %q", r)
	}
	if strings.Contains(r, "r1") || strings.Contains(r, "r4") {
		t.Errorf("collapsed tool must hide its output, got %q", r)
	}
}

// TestContentBlock_RenderToolExpandedGlyph — an expanded tool with output
// shows the ▾ glyph so it reads as collapsible.
func TestContentBlock_RenderToolExpandedGlyph(t *testing.T) {
	b := ContentBlock{Type: BlockTool, ToolName: "search", ToolDone: true, Text: "out"}
	if r := b.Render(); !strings.Contains(r, "▾") {
		t.Errorf("expanded tool with output should show the ▾ glyph, got %q", r)
	}
}

// TestContentBlock_RenderToolOutputFenced — multi-line tool output is wrapped
// in a fenced block so the markdown renderer keeps its line breaks instead of
// merging consecutive blockquote lines into one paragraph.
func TestContentBlock_RenderToolOutputFenced(t *testing.T) {
	b := ContentBlock{Type: BlockTool, ToolName: "ls", ToolDone: true, Text: "a/\nb/\nc/"}
	r := b.Render()
	if !strings.Contains(r, "~~~") {
		t.Errorf("tool output should be wrapped in a fenced block, got %q", r)
	}
}

// TestContentBlock_RenderToolArgsWithBacktickDoesNotBreakOut
// regression-guards: ToolArgs was interpolated into a single-backtick
// inline code span with no escaping — a backtick inside the args (e.g. a
// search string or shell snippet an agent passed as a tool argument)
// terminated the span early, letting whatever followed be parsed as normal
// Markdown instead of literal code.
func TestContentBlock_RenderToolArgsWithBacktickDoesNotBreakOut(t *testing.T) {
	b := ContentBlock{Type: BlockTool, ToolName: "search", ToolArgs: "a `backtick` in the middle", ToolDone: false}
	r := b.Render()
	// Render() returns Markdown SOURCE, not glamour-rendered output, so
	// checking that the raw args substring appears somewhere in it proves
	// nothing — it's always there either way. What actually matters is the
	// code-span DELIMITER: it must be longer than the longest backtick run
	// inside the content (1, here), or the embedded backtick terminates a
	// plain single-backtick span early once glamour parses it.
	if !strings.Contains(r, "``") {
		t.Fatalf("code-span delimiter should be bumped past the content's own single backtick (need at least 2 backticks as the actual delimiter), got %q", r)
	}
}

// TestContentBlock_RenderToolOutputWithTildeFenceDoesNotBreakOut
// regression-guards: tool output was always wrapped in a fixed 3-tilde
// fence — output that itself contains a "~~~" run (a diff, a file using
// tildes as separators, nested markdown) terminated the fence early,
// letting the rest of the output be parsed as normal Markdown instead of
// literal text.
func TestContentBlock_RenderToolOutputWithTildeFenceDoesNotBreakOut(t *testing.T) {
	b := ContentBlock{Type: BlockTool, ToolName: "cat", ToolDone: true, Text: "before\n~~~\nafter"}
	r := b.Render()
	if !strings.Contains(r, "~~~~") {
		t.Fatalf("fence should be bumped past the content's own 3-tilde run (need at least 4 tildes as the actual delimiter), got %q", r)
	}
	if !strings.Contains(r, "before") || !strings.Contains(r, "~~~") || !strings.Contains(r, "after") {
		t.Errorf("output content should still be present intact, got %q", r)
	}
}
