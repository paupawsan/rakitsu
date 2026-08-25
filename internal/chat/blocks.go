package chat

import (
	"fmt"
	"strings"
)

// BlockType identifies the kind of content block.
type BlockType int

const (
	BlockUser BlockType = iota
	BlockAssistant
	BlockTool
	BlockSystem
	BlockReasoning
	BlockSubagent
)

// ContentBlock represents a rendered segment in the chat viewport.
type ContentBlock struct {
	Type     BlockType
	Text     string // accumulated text (markdown for assistant, plain for others)
	ToolID   string // for tool blocks
	ToolName string
	ToolArgs string
	ToolDone bool
	ToolErr  string
	Duration int64 // ms

	// Subagent-block fields (BlockSubagent — a runtime-spawned child's live
	// lifecycle row). AgentName below carries the child's instance name.
	SubModel  string // child's model, shown only while running
	SubStatus string // AgentEndPayload.Status once SubDone ("success", "timeout", ...)
	SubTokens int
	SubDone   bool

	// AgentName is the agent that emitted this block (tool/reasoning blocks).
	// Depth is the nesting indent: 0 for the root agent, 1+ for sub-agents
	// reached via delegation. Together they drive tool-call nesting.
	AgentName string
	Depth     int

	// Collapsed hides a tool/reasoning block's body, leaving a one-line
	// summary. Set automatically (old reasoning auto-collapses) and toggled
	// by a mouse click on the block.
	Collapsed bool

	// Phase 2 — branching turn metadata. TurnID groups blocks by the user
	// turn that produced them; BranchIndex/BranchCount are populated on
	// user blocks (kind == BlockUser) and drive the `‹n/m›` chip that
	// signals other branches exist for this turn. Left zero in the
	// current flat-blocks TUI; populated when PR E-2 wires
	// turntree.Tree[ContentBlock] into model.go.
	TurnID      string
	BranchIndex int
	BranchCount int
}

// lineCountLabel renders a singular/plural "N line(s)" label.
func lineCountLabel(n int) string {
	if n == 1 {
		return "1 line"
	}
	return fmt.Sprintf("%d lines", n)
}

// longestRun returns the length of the longest consecutive run of r in s.
func longestRun(s string, r rune) int {
	longest, cur := 0, 0
	for _, c := range s {
		if c == r {
			cur++
			if cur > longest {
				longest = cur
			}
		} else {
			cur = 0
		}
	}
	return longest
}

// inlineCode wraps text in a Markdown inline code span using enough
// backticks to survive any backtick run already inside text — a plain
// single-backtick span (the common case) terminates early on the first
// backtick that arbitrary content (a tool argument, in practice) happens to
// contain, letting whatever follows be parsed as normal Markdown instead of
// literal code. A leading/trailing space pads the span per CommonMark's
// rule for content that starts or ends with a backtick itself.
func inlineCode(text string) string {
	fence := strings.Repeat("`", longestRun(text, '`')+1)
	if text == "" || strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		return fence + " " + text + " " + fence
	}
	return fence + text + fence
}

// tildeFence returns a Markdown fence delimiter (minimum 3 tildes) longer
// than the longest run of "~" anywhere in content — content containing a
// run as long as or longer than a fixed 3-tilde fence (a diff, a file that
// uses tildes as separators) would otherwise terminate the fence early,
// letting the rest of the content be parsed as normal Markdown instead of
// literal text. content may be multi-line; newlines naturally break a run
// the same way any other non-"~" character does.
func tildeFence(content string) string {
	n := longestRun(content, '~') + 1
	if n < 3 {
		n = 3
	}
	return strings.Repeat("~", n)
}

// Render produces the display string for this block.
// Blocks are separated by blank lines so glamour treats each as its
// own paragraph (single \n is merged into one paragraph in markdown).
func (b *ContentBlock) Render() string {
	switch b.Type {
	case BlockUser:
		// Render user input as a blockquote so markdown preserves line
		// breaks (hard-breaks via "  \n" are unreliable through glamour)
		// and the block is visually distinct from assistant text.
		var sb strings.Builder
		sb.WriteString("\n**You:**")
		// Branch chip — only when this user turn has siblings (set by
		// PR E-2's turntree adoption; zero in the current flat-blocks
		// path). Goes on the header line so it doesn't disrupt the
		// blockquote.
		if b.BranchCount > 1 {
			sb.WriteString(fmt.Sprintf(" `‹%d/%d›`", b.BranchIndex+1, b.BranchCount))
		}
		sb.WriteString("\n")
		for _, line := range strings.Split(b.Text, "\n") {
			sb.WriteString("> ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
		return sb.String()
	case BlockAssistant:
		if b.Text == "" {
			return ""
		}
		return fmt.Sprintf("%s\n\n", b.Text)
	case BlockTool:
		return b.renderTool()
	case BlockReasoning:
		return b.renderReasoning()
	case BlockSubagent:
		return b.renderSubagent()
	case BlockSystem:
		// Single-line system messages render as italic. Multi-line text is
		// wrapped in a tilde-fence instead — glamour collapses single \n
		// inside italic into soft-wraps, mashing list output onto one line
		// (same root cause as gotcha_glamour_blockquote_line_merge for
		// blockquoted tool output).
		if strings.Contains(b.Text, "\n") {
			fence := tildeFence(b.Text)
			return fmt.Sprintf("\n%s\n%s\n%s\n\n", fence, b.Text, fence)
		}
		return fmt.Sprintf("\n*%s*\n\n", b.Text)
	default:
		return b.Text + "\n\n"
	}
}

// quotePrefix returns the blockquote marker for this block's depth. Depth 0
// yields "> "; each deeper level nests another blockquote so glamour indents
// sub-agent activity further than the root agent's.
func (b *ContentBlock) quotePrefix() string {
	return strings.Repeat("> ", b.Depth+1)
}

func (b *ContentBlock) renderTool() string {
	quote := b.quotePrefix()
	var sb strings.Builder
	// For sub-agent tool calls (depth > 0) prefix the agent name so the
	// nesting is legible even where blockquote indentation is subtle.
	name := b.ToolName
	if b.Depth > 0 && b.AgentName != "" {
		name = fmt.Sprintf("%s → %s", b.AgentName, b.ToolName)
	}

	// A completed tool with output is collapsible — show a ▸/▾ glyph so the
	// block reads as clickable.
	hasBody := b.ToolDone && b.Text != ""
	glyph := ""
	if hasBody {
		if b.Collapsed {
			glyph = "▸ "
		} else {
			glyph = "▾ "
		}
	}

	sb.WriteString(fmt.Sprintf("\n%s%s**%s**", quote, glyph, name))
	if b.ToolArgs != "" {
		// Show compact args
		args := previewLine(b.ToolArgs, 77)
		sb.WriteString(" " + inlineCode(args))
	}
	if !b.ToolDone {
		sb.WriteString("  ⏳")
	} else if b.ToolErr != "" {
		sb.WriteString(fmt.Sprintf("  ✗ %s", b.ToolErr))
	} else {
		sb.WriteString(fmt.Sprintf("  ✓ %dms", b.Duration))
	}

	if !hasBody {
		sb.WriteString("\n")
		return sb.String()
	}

	lines := strings.Split(strings.TrimRight(b.Text, "\n"), "\n")
	if b.Collapsed {
		// Header line only, with a line-count summary of the hidden output.
		sb.WriteString(fmt.Sprintf("  · %s\n", lineCountLabel(len(lines))))
		return sb.String()
	}
	sb.WriteString("\n")

	// Tool output goes in a fenced code block so multi-line output (file
	// listings, grep results) keeps its line breaks — consecutive blockquote
	// lines would otherwise be merged into one paragraph by the markdown
	// renderer. A tilde fence survives output that itself contains ``` .
	maxLines := 6
	shown, more := lines, 0
	if len(lines) > maxLines {
		shown, more = lines[:maxLines], len(lines)-maxLines
	}
	fence := tildeFence(strings.Join(shown, "\n"))
	sb.WriteString(quote + fence + "\n")
	for _, line := range shown {
		sb.WriteString(quote + line + "\n")
	}
	sb.WriteString(quote + fence + "\n")
	if more > 0 {
		sb.WriteString(fmt.Sprintf("%s*... (%d more lines)*\n", quote, more))
	}
	return sb.String()
}

// fmtSubTokens renders a token count the way the status bar's existing
// counter reads: compact "3.0K tok" above 1000, plain "N tok" below.
func fmtSubTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fK tok", float64(n)/1000)
	}
	return fmt.Sprintf("%d tok", n)
}

// renderSubagent shows a runtime-spawned child's lifecycle as one line:
// a pulsing-dot status while running, a ✓/✗ summary once done. Distinct
// glyph set from tool blocks (◐/✓/✗ vs ⏳/✓/✗) so subagents read as agents,
// not tool calls, at a glance.
func (b *ContentBlock) renderSubagent() string {
	quote := b.quotePrefix()
	name := b.AgentName
	if !b.SubDone {
		return fmt.Sprintf("\n%s◐ **%s** · %s · working…\n", quote, name, b.SubModel)
	}
	if b.SubStatus != "" && b.SubStatus != "success" {
		return fmt.Sprintf("\n%s✗ **%s** · %s\n", quote, name, b.SubStatus)
	}
	return fmt.Sprintf("\n%s✓ **%s** · %s · %.1fs\n", quote, name, fmtSubTokens(b.SubTokens), float64(b.Duration)/1000)
}

// renderReasoning shows streaming reasoning-model CoT as a dimmed blockquote.
// Collapsed, it is a one-line summary; expanded, the body is tail-truncated —
// the most recent lines are the useful progress signal mid-thought.
func (b *ContentBlock) renderReasoning() string {
	if strings.TrimSpace(b.Text) == "" {
		return ""
	}
	quote := b.quotePrefix()
	var sb strings.Builder
	lines := strings.Split(strings.TrimRight(b.Text, "\n"), "\n")

	if b.Collapsed {
		// A finished thought, summarized — the ▸ glyph signals click-to-expand.
		label := "💭 *thought*"
		if b.Depth > 0 && b.AgentName != "" {
			label = fmt.Sprintf("💭 *%s thought*", b.AgentName)
		}
		sb.WriteString(fmt.Sprintf("\n%s▸ %s · %s\n\n", quote, label, lineCountLabel(len(lines))))
		return sb.String()
	}

	label := "💭 *thinking*"
	if b.Depth > 0 && b.AgentName != "" {
		label = fmt.Sprintf("💭 *%s thinking*", b.AgentName)
	}
	sb.WriteString("\n" + quote + "▾ " + label + "\n")

	const maxLines = 12
	if len(lines) > maxLines {
		sb.WriteString(fmt.Sprintf("%s*... (%d earlier lines)*\n", quote, len(lines)-maxLines))
		lines = lines[len(lines)-maxLines:]
	}
	for _, line := range lines {
		sb.WriteString(quote + line + "\n")
	}
	sb.WriteString("\n")
	return sb.String()
}
