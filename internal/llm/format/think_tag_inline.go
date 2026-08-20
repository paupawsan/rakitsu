package format

import (
	"regexp"
	"strings"
)

// ThinkTagInline handles models that emit chain-of-thought inside
// <think>...</think> blocks in the normal content stream. Examples:
// Qwen3 (thinking variants), DeepSeek-R1 running on non-vLLM deployments
// that don't separate content and reasoning_content, and nemotron-elastic-30b
// under vLLM deployments that don't enable --reasoning-parser.
//
// Behavior:
//
//   - Streaming (ApplyDelta): runs an incremental <think> parser that splits
//     each delta into a "content" portion (passed to the inner adapter
//     verbatim) and a "reasoning" portion (accumulated into state.Reasoning
//     and emitted live via state.OnReasoningDelta when set). Partial tags
//     spanning multiple deltas are held in scratch state until resolved.
//
//   - Non-streaming (ApplyFull): delegates to the inner adapter, then
//     Finalize runs a regex post-pass over the accumulated content to
//     extract any remaining <think>...</think> blocks. Same regex
//     extraction also runs as a belt-and-braces safety net after streaming.
//
// ThinkTagInline is a POST-PROCESSING adapter: it owns the <think> split
// and delegates everything else (content accumulation, tool-call extraction,
// reasoning_content handling) to the inner adapter (StandardOpenAI by default).
type ThinkTagInline struct {
	// Inner is the adapter that handles the raw delta/full processing before
	// this adapter post-processes the result. If nil, defaults to StandardOpenAI.
	Inner ResponseFormat
}

func (ThinkTagInline) Name() string { return "think_tag_inline" }

// thinkTagRegex matches <think>...</think> blocks. Identical to the original
// thinkTagRe in openai/provider.go; kept at package scope in the format
// package so the regex is compiled once.
var thinkTagRegex = regexp.MustCompile(`(?s)<think>(.*?)</think>`)

const (
	openThinkTag  = "<think>"
	closeThinkTag = "</think>"
)

// thinkInlineState is the per-request scratch for ThinkTagInline's streaming
// parser. Tracks how deeply nested we are inside <think> blocks (0 = outside),
// holds back trailing bytes that might be a partial tag, and accumulates the
// current block's raw bytes so we can commit them (trimmed) to
// state.Reasoning when the outermost closing tag arrives.
type thinkInlineState struct {
	depth           int             // nesting depth of <think> blocks; 0 = outside
	tagBuffer       string          // bytes held back from emission; potential partial tag
	curBlock        strings.Builder // raw bytes of the currently-open <think> block(s)
	committedBlocks int             // number of <think> blocks the streaming parser has fully extracted
}

// getThinkInlineState lazily initializes the scratch slot on FormatState.
// Safe to call multiple times.
func getThinkInlineState(state *FormatState) *thinkInlineState {
	if state.thinkInline == nil {
		state.thinkInline = &thinkInlineState{}
	}
	return state.thinkInline
}

func (t ThinkTagInline) inner() ResponseFormat {
	if t.Inner != nil {
		return t.Inner
	}
	return StandardOpenAI{}
}

func (t ThinkTagInline) ApplyDelta(state *FormatState, delta RawDelta) string {
	ts := getThinkInlineState(state)
	cleanContent := ts.consume(delta.Content, state)

	// Forward to inner with cleaned content but original reasoning_content,
	// tool_calls, and finish_reason so inner adapters (e.g. ReasoningContentField)
	// continue to see the fields they care about untouched.
	return t.inner().ApplyDelta(state, RawDelta{
		Content:          cleanContent,
		ReasoningContent: delta.ReasoningContent,
		ToolCalls:        delta.ToolCalls,
		FinishReason:     delta.FinishReason,
	})
}

func (t ThinkTagInline) ApplyFull(state *FormatState, msg RawMessage) {
	t.inner().ApplyFull(state, msg)
}

func (t ThinkTagInline) Finalize(state *FormatState) Result {
	// Flush any open <think> block from the streaming parser. A model that
	// never closed its think tag still produced reasoning bytes we should
	// preserve in state.Reasoning.
	if state.thinkInline != nil {
		state.thinkInline.flush(state)
	}

	r := t.inner().Finalize(state)

	// Belt-and-braces: if any <think> tags survived (ApplyFull path, or a
	// streaming case where the parser somehow missed bytes), strip them now.
	thinking, clean := extractThinkTags(r.Content)
	if thinking != "" {
		r.Content = clean
		if r.Reasoning == "" {
			r.Reasoning = thinking
		} else {
			r.Reasoning = r.Reasoning + "\n\n" + thinking
		}
	} else if state.thinkInline != nil && state.thinkInline.committedBlocks > 0 {
		// Streaming parser already stripped the tags from r.Content. Match
		// the regex-path's TrimSpace behavior so a leading or trailing
		// newline left over from the stripped block doesn't reach the user.
		r.Content = strings.TrimSpace(r.Content)
	}
	return r
}

// consume processes one delta's content bytes through the <think> state
// machine and returns the bytes that should be forwarded to the inner adapter
// as Content. Reasoning bytes are accumulated into curBlock (committed to
// state.Reasoning on </think>) and emitted live via state.OnReasoningDelta
// when set.
//
// The parser holds back trailing bytes that could form a partial tag so a
// boundary like "<think>foo</thi" + "nk>bar" produces reasoning="foo" and
// content="bar", not a corrupted mix.
func (ts *thinkInlineState) consume(input string, state *FormatState) string {
	buf := ts.tagBuffer + input
	ts.tagBuffer = ""
	var content strings.Builder

	for {
		if ts.depth == 0 {
			idx := strings.Index(buf, openThinkTag)
			if idx >= 0 {
				content.WriteString(buf[:idx])
				buf = buf[idx+len(openThinkTag):]
				ts.depth = 1
				continue
			}
			// No full opening tag. Hold back any suffix that could be a
			// partial tag; flush the rest as content.
			partial := longestPartialPrefix(buf, openThinkTag)
			if partial > 0 {
				content.WriteString(buf[:len(buf)-partial])
				ts.tagBuffer = buf[len(buf)-partial:]
			} else {
				content.WriteString(buf)
			}
			return content.String()
		}

		// Inside a think block: a nested <think> increments depth (still
		// reasoning, not a real close), a </think> decrements it — only the
		// one that brings depth back to 0 is the real, outermost close.
		// Whichever of the two appears first in the buffer is handled first.
		openIdx := strings.Index(buf, openThinkTag)
		closeIdx := strings.Index(buf, closeThinkTag)

		if closeIdx >= 0 && (openIdx < 0 || closeIdx < openIdx) {
			ts.emitReasoning(buf[:closeIdx], state)
			buf = buf[closeIdx+len(closeThinkTag):]
			ts.depth--
			if ts.depth == 0 {
				ts.commitBlock(state)
			}
			continue
		}
		if openIdx >= 0 {
			ts.emitReasoning(buf[:openIdx], state)
			buf = buf[openIdx+len(openThinkTag):]
			ts.depth++
			continue
		}
		// Neither a full close nor a full nested-open tag found; hold back
		// any suffix that could be a partial tag of either kind.
		partial := longestPartialPrefix(buf, closeThinkTag)
		if p := longestPartialPrefix(buf, openThinkTag); p > partial {
			partial = p
		}
		if partial > 0 {
			ts.emitReasoning(buf[:len(buf)-partial], state)
			ts.tagBuffer = buf[len(buf)-partial:]
		} else {
			ts.emitReasoning(buf, state)
		}
		return content.String()
	}
}

// emitReasoning appends raw reasoning bytes to the current-block buffer and
// forwards them live via state.OnReasoningDelta when set. Empty inputs are
// a no-op so consumers don't see zero-length chunks.
func (ts *thinkInlineState) emitReasoning(s string, state *FormatState) {
	if s == "" {
		return
	}
	ts.curBlock.WriteString(s)
	if state.OnReasoningDelta != nil {
		state.OnReasoningDelta(s)
	}
}

// commitBlock writes the current-block's TrimSpace'd content to state.Reasoning,
// separated from prior blocks by "\n\n" to match the regex extractThinkTags
// output format. Called on each </think> close.
func (ts *thinkInlineState) commitBlock(state *FormatState) {
	block := strings.TrimSpace(ts.curBlock.String())
	ts.curBlock.Reset()
	if block == "" {
		return
	}
	if state.Reasoning.Len() > 0 {
		state.Reasoning.WriteString("\n\n")
	}
	state.Reasoning.WriteString(block)
	ts.committedBlocks++
}

// flush is called from Finalize to commit any reasoning that arrived inside
// a never-closed <think> block. Also flushes the tagBuffer's contents — if
// the stream ended with partial-tag bytes, they should appear as content
// (or reasoning, if we were inside think) rather than being silently dropped.
func (ts *thinkInlineState) flush(state *FormatState) {
	if ts.tagBuffer != "" {
		if ts.depth > 0 {
			ts.emitReasoning(ts.tagBuffer, state)
		}
		// Bytes that would have been content land in state.Content via the
		// next-delta path that never came; for symmetry we don't synthesize
		// a phantom inner.ApplyDelta call here — leftover partial-open-tag
		// bytes outside think are dropped, matching pre-B54 behavior where
		// the regex strip silently consumed them too.
		ts.tagBuffer = ""
	}
	if ts.curBlock.Len() > 0 {
		ts.commitBlock(state)
	}
}

// longestPartialPrefix returns the largest n such that buf's last n bytes
// match needle's first n bytes (and n < len(needle), since a full match
// would be returned by strings.Index instead). Used to hold back trailing
// bytes that might continue into a tag on the next delta.
func longestPartialPrefix(buf, needle string) int {
	maxN := len(needle) - 1
	if maxN > len(buf) {
		maxN = len(buf)
	}
	for n := maxN; n > 0; n-- {
		if strings.HasSuffix(buf, needle[:n]) {
			return n
		}
	}
	return 0
}

// extractThinkTags strips <think>...</think> blocks from s and returns the
// concatenated thinking text plus the cleaned content. Behavior mirrors the
// original extractOpenAIThinking helper in openai/provider.go (preserved
// during Phase 2 migration to avoid behavior drift).
//
// Used by Finalize as a belt-and-braces safety net for the ApplyFull path
// and for any edge case where the streaming parser missed a tag. Returns
// thinking="" and clean=s untouched when no tags are present.
func extractThinkTags(s string) (thinking, clean string) {
	matches := thinkTagRegex.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return "", s
	}
	parts := make([]string, 0, len(matches))
	for _, m := range matches {
		parts = append(parts, strings.TrimSpace(m[1]))
	}
	thinking = strings.Join(parts, "\n\n")
	clean = strings.TrimSpace(thinkTagRegex.ReplaceAllString(s, ""))
	return thinking, clean
}
