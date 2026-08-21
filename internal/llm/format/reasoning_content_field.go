package format

import (
	"regexp"
	"strings"
)

// ReasoningContentField handles reasoning models that return chain-of-thought
// on a separate `reasoning_content` field (streaming and non-streaming).
// Examples: Nemotron-30b, DeepSeek-R1, QwQ via vLLM/LiteLLM.
//
// Behavior:
//   - reasoning_content deltas accumulate into state.Reasoning (silently — not
//     surfaced to the tracer, so the live output stays clean)
//   - content deltas accumulate into state.Content as normal
//   - On Finalize, if content is empty but reasoning is populated, the adapter
//     attempts to extract a "final answer" section from the reasoning text
//     (e.g. markdown report, explicit "Thus:" marker, or trailing structured
//     block). This is the Gate 2 unblocker for small-context reasoning models
//     whose entire completion budget gets consumed by reasoning before any
//     content chunk is emitted.
//   - If extraction fails, the whole reasoning text is promoted to content with
//     a [REASONING-ONLY OUTPUT — ...] marker prefix so the operator sees that
//     the output is raw CoT, not a real answer.
type ReasoningContentField struct{}

func (ReasoningContentField) Name() string { return "reasoning_content_field" }

func (r ReasoningContentField) ApplyDelta(state *FormatState, delta RawDelta) string {
	// Reuse StandardOpenAI's content + tool-call handling — this adapter only
	// differs in how it handles reasoning_content.
	surface := StandardOpenAI{}.ApplyDelta(state, RawDelta{
		Content:      delta.Content,
		ToolCalls:    delta.ToolCalls,
		FinishReason: delta.FinishReason,
	})

	if delta.ReasoningContent != "" {
		state.Reasoning.WriteString(delta.ReasoningContent)
	}

	return surface
}

func (r ReasoningContentField) ApplyFull(state *FormatState, msg RawMessage) {
	StandardOpenAI{}.ApplyFull(state, RawMessage{
		Content:      msg.Content,
		ToolCalls:    msg.ToolCalls,
		FinishReason: msg.FinishReason,
	})
	if msg.ReasoningContent != "" {
		state.Reasoning.WriteString(msg.ReasoningContent)
	}
}

func (r ReasoningContentField) Finalize(state *FormatState) Result {
	content := state.Content.String()
	reasoning := state.Reasoning.String()
	toolCalls := assembleToolCalls(state)

	// Salvaged: true when Content is the marker-prefix fallback rather
	// than a real model commitment. Consumed by the agent loop's
	// Mitigation A path to avoid treating reasoning-only output as
	// a successful final answer.
	salvaged := false

	// Content-stream-empty case: try to extract a final answer from reasoning.
	// Only applies when there are no tool calls — if the model is asking to
	// call a tool, there's no "final answer" to extract.
	if content == "" && reasoning != "" && len(toolCalls) == 0 {
		if extracted, ok := extractFinalAnswerWithLabels(reasoning, state.Labels); ok {
			content = extracted
		} else {
			// Extraction failed — promote full reasoning with a warning marker.
			content = reasoningOnlyMarker + reasoning
			salvaged = true
		}
	}

	return Result{
		Content:      content,
		Reasoning:    reasoning,
		ToolCalls:    toolCalls,
		FinishReason: state.FinishReason,
		Salvaged:     salvaged,
	}
}

const reasoningOnlyMarker = "[REASONING-ONLY OUTPUT — the model did not emit a content stream; what follows is its internal chain-of-thought and may contain hallucinated continuations. Consider lowering temperature, raising max_tokens, or switching to a non-reasoning model.]\n\n"

// finalAnswerMarkers matches explicit final-answer markers that reasoning
// models commonly emit near the end of their chain-of-thought. Tried in
// order, most specific first, and each pattern is searched independently —
// NOT combined into one alternation. Go's regexp finds the leftmost match
// *position* in the input, so a single combined alternation would let a
// weak marker (e.g. "thus,") appearing earlier in the text pre-empt a
// stronger marker (e.g. "final answer:") appearing later, even though the
// alternation lists the stronger one first — alternation order only breaks
// ties at the same starting position, it doesn't rank matches by position.
// Trying each pattern separately and taking the first PATTERN (not the
// first text position) that matches anywhere restores the intended
// most-specific-wins semantics.
//
// Patterns intentionally case-insensitive and tolerant of surrounding punctuation.
// Each capture group 1 is the text AFTER the marker, which we treat as the
// final answer candidate.
var finalAnswerMarkers = []*regexp.Regexp{
	regexp.MustCompile(`(?is)final\s+answer\s*[:\-]\s*(.+)$`),
	regexp.MustCompile(`(?is)final\s+output\s*[:\-]\s*(.+)$`),
	regexp.MustCompile(`(?is)final\s+report\s*[:\-]\s*(.+)$`),
	regexp.MustCompile(`(?is)final\s+response\s*[:\-]\s*(.+)$`),
	regexp.MustCompile(`(?is)final\s+result\s*[:\-]\s*(.+)$`),
	regexp.MustCompile(`(?is)the\s+final\s+(?:answer|output|report|response|result)\s+is\s*[:\-]?\s*(.+)$`),
	regexp.MustCompile(`(?is)here\s+is\s+the\s+(?:final\s+)?(?:answer|output|report|response|result)\s*[:\-]?\s*(.+)$`),
	regexp.MustCompile(`(?is)thus\s*[,:]\s*(.+)$`),
	regexp.MustCompile(`(?is)therefore\s*[,:]\s*(.+)$`),
	regexp.MustCompile(`(?is)so\s+the\s+(?:final\s+)?(?:answer|output|report|response|result)\s+is\s*[:\-]?\s*(.+)$`),
}

// markdownReportRe matches markdown report blocks that look like the actual
// deliverable — useful when the model produces structured output embedded in
// reasoning without an explicit "final answer" marker.
//
// A "report" is heuristically a block that starts with a top-level markdown
// header (## or #) and contains at least one bullet point or second header.
var markdownReportRe = regexp.MustCompile(
	`(?s)(#{1,3}\s+\S.*?)$`,
)

// looseImperativeMarkerRe is the Phase 6 Strategy 5 regex: it matches
// "thus / therefore / so" followed by a COMMITTED subject + verb pattern
// (subject has determined / answer is / answer becomes / I will return …)
// — the linguistic shape reasoning models use to introduce a final answer
// without the colon/comma the strict finalAnswerMarkers patterns demand.
//
// Verbs are deliberately restricted to "committed" forms (have, is, are,
// becomes, will be) and "I will <return-like>" because "intent" verbs
// (need, should, must, we will) are dominantly deliberative in CoT —
// e.g. "Thus we need to output X" is the model talking about the answer,
// not committing to one. See TestExtractFinalAnswer_Strategy5_RejectsMetaCommentary.
//
// Examples it catches:
//
//	"Thus we have determined that …"
//	"Therefore the answer becomes …"
//	"So I will return …"
//
// Examples it correctly skips:
//
//	"Thus we need to output these lines …"   (still planning)
//	"So we will need to reconsider …"        (self-correction)
//
// The capture is everything AFTER the marker phrase. Candidates MUST also
// pass looksLikeOutput before being returned as the final answer (catches
// the residual case where the regex matches but the captured text drifts
// into meta-commentary, e.g. "Therefore I have to find …. Let me think").
var looseImperativeMarkerRe = regexp.MustCompile(
	`(?is)` +
		`(?:` +
		`thus\s+(?:i\s+(?:will|have)\s+|we\s+have\s+|the\s+(?:answer|result|output|report|response)\s+(?:is|are|becomes?|will\s+be)\s+)|` +
		`therefore\s+(?:i\s+(?:will|have)\s+|we\s+have\s+|the\s+(?:answer|result|output|report|response)\s+(?:is|are|becomes?|will\s+be)\s+)|` +
		`so\s+(?:i\s+will\s+(?:return|provide|emit|output|give|report|send)\s+|the\s+(?:answer|result|output|report|response)\s+(?:becomes?|will\s+be)\s+)` +
		`)(.+)$`,
)

// metaCommentaryPhrases are CoT-state markers that indicate the model is
// still planning or reconsidering, NOT committing to an answer. A candidate
// containing any of these is rejected by looksLikeOutput because it's
// reasoning-about-the-answer, not the answer itself.
//
// All checks are case-insensitive on the candidate.
var metaCommentaryPhrases = []string{
	"we need to ",
	"we should ",
	"we have to ",
	"we'll need to ",
	"we'll have to ",
	"i need to ",
	"i should ",
	"i'll need to ",
	"let me ",
	"let us ",
	"but wait",
	"actually,",
	"actually ",
	"hmm,",
	"hmm ",
	"wait,",
	"wait ",
	"let me reconsider",
	"let me think",
	"on second thought",
}

// sentenceTerminatorRe counts roughly-sentence-ending punctuation — used as
// a crude length heuristic in looksLikeOutput. Doesn't try to be a real
// sentence segmenter.
var sentenceTerminatorRe = regexp.MustCompile(`[.!?](?:\s|$)`)

// looksLikeOutput reports whether a candidate string looks like a real model
// output rather than meta-commentary. Used by Strategy 5 (loose imperative
// marker) to suppress false positives where the matched phrase introduced
// further deliberation, not a committed answer.
//
// Rules (in order):
//  1. Empty or whitespace-only → false.
//  2. Contains any metaCommentaryPhrases → false (model still planning).
//  3. Short (≤ 250 chars and ≤ 3 sentences) and no meta phrases → true
//     (clean concise conclusion).
//  4. Long → require at least one structural marker: ≥ 2 bullet/numbered
//     list items, OR ≥ 1 markdown header, OR a fenced code block.
func looksLikeOutput(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}

	lower := strings.ToLower(s)
	for _, p := range metaCommentaryPhrases {
		if strings.Contains(lower, p) {
			return false
		}
	}

	if len(s) <= 250 && len(sentenceTerminatorRe.FindAllString(s, -1)) <= 3 {
		return true
	}

	bulletCount, headerCount := 0, 0
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") {
			bulletCount++
			continue
		}
		if strings.HasPrefix(t, "# ") || strings.HasPrefix(t, "## ") || strings.HasPrefix(t, "### ") {
			headerCount++
			continue
		}
		// Numbered list items: "1. ", "12. ", etc.
		if len(t) >= 3 && t[0] >= '0' && t[0] <= '9' {
			rest := t[1:]
			for len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
				rest = rest[1:]
			}
			if len(rest) >= 2 && rest[0] == '.' && rest[1] == ' ' {
				bulletCount++
			}
		}
	}
	if bulletCount >= 2 || headerCount >= 1 {
		return true
	}
	if strings.Contains(s, "```") {
		return true
	}
	return false
}

// extractFinalAnswer is the no-labels shorthand for extractFinalAnswerWithLabels.
// Used by call sites that don't have prompt-derived labels available.
// Preserves the API used by existing tests.
func extractFinalAnswer(reasoning string) (string, bool) {
	return extractFinalAnswerWithLabels(reasoning, nil)
}

// extractFinalAnswerWithLabels attempts to pull a clean final answer out of
// reasoning text. When `labels` is non-empty, Strategy 4 (label-prefix
// harvest) is enabled as a high-priority opt-in path.
//
// Strategy (first hit wins):
//  1. Explicit strict marker: "Final answer:", "Thus:", "Therefore:" etc.
//  4. Label-prefix harvest (Phase 6, opt-in): line-anchored "LABEL: value"
//     extraction for each provided label. Requires ≥ half labels matched
//     (or all, when ≤ 3 labels) to avoid stealing on partial scrap matches.
//     Runs early because explicit user opt-in is a stronger signal than
//     any heuristic structural detection.
//  2. Trailing markdown report block (## or ### headers at end of reasoning).
//  3. Trailing structured-list block (Phase 6): a bullet or numbered list of
//     ≥ 5 contiguous items, the last such block whose end is in the trailing
//     50% of the reasoning.
//  5. Loose imperative marker (Phase 6): "Thus we …", "Therefore the answer
//     becomes …", "So I will …" — candidate must pass looksLikeOutput shape
//     validation (rejects meta-commentary like "Thus we need to think more").
//     -. Fail — caller will fall back to the reasoning-only marker prefix.
//
// Strategy ordering rationale:
//   - Strategy 1 is strongest (explicit marker).
//   - Strategy 4 runs second when labels are provided because user opt-in
//     is a stronger signal than any heuristic. Falls through naturally
//     below the min-match threshold.
//   - Strategy 2 beats Strategy 3 because a markdown report with headers is
//     more deliberate "I'm writing the output now" signal than a bare list.
//   - Strategy 3 beats Strategy 5 because a structured list anchored in the
//     trailing half of the text is more reliably output than a sentence-
//     level loose-marker match anywhere in the text.
//
// Extraction is deliberately conservative: if we're not confident the
// extracted text is a real answer, we fail and let the caller surface the
// full reasoning with a warning. False positives (extracting garbage and
// calling it the answer) are worse than false negatives.
func extractFinalAnswerWithLabels(reasoning string, labels []string) (string, bool) {
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return "", false
	}

	// Strategy 1: explicit strict marker. Try each pattern in priority order
	// (most specific first) rather than one combined alternation — see
	// finalAnswerMarkers' doc comment for why that distinction matters.
	for _, re := range finalAnswerMarkers {
		if m := re.FindStringSubmatch(reasoning); m != nil && len(m) >= 2 {
			candidate := strings.TrimSpace(m[1])
			if len(candidate) >= 20 { // avoid extracting "Thus: done." as an answer
				return candidate, true
			}
		}
	}

	// Strategy 4: label-prefix harvest (opt-in via non-empty labels).
	if len(labels) > 0 {
		if extracted, ok := extractByLabelPrefixes(reasoning, labels); ok {
			return extracted, true
		}
	}

	// Strategy 2: trailing markdown report. Find the LAST section-level header
	// (##/###) in the reasoning — that's the start of the final synthesis, not
	// a quote of a source the model was reading. Going from-last-header-forward
	// reliably captures the model's own output and rejects earlier sections
	// that might be tool-result quotes embedded in chain-of-thought.
	if idx := findTrailingMarkdownReport(reasoning); idx >= 0 {
		candidate := strings.TrimSpace(reasoning[idx:])
		if len(candidate) >= 40 && strings.Count(candidate, "\n") >= 2 {
			return candidate, true
		}
	}

	// Strategy 3: trailing structured-list. Find the last qualifying
	// contiguous bullet/numbered list anchored in the trailing 50% of the
	// reasoning.
	if extracted, ok := extractTrailingStructuredList(reasoning); ok {
		return extracted, true
	}

	// Strategy 5: loose imperative marker with shape validation. Catches
	// "Thus we have determined …" / "Therefore the answer becomes …" /
	// "So I will return …" — patterns the strict regex misses because they
	// substitute whitespace + verb for the punctuation it requires.
	if m := looseImperativeMarkerRe.FindStringSubmatch(reasoning); m != nil && len(m) >= 2 {
		candidate := strings.TrimSpace(m[1])
		if len(candidate) >= 20 && looksLikeOutput(candidate) {
			return candidate, true
		}
	}

	return "", false
}

// labelPrefixRe builds a regex that line-anchored matches any of the given
// labels at the start of a line, followed by ":". Labels are regex-escaped
// before composition.
func labelPrefixRe(labels []string) *regexp.Regexp {
	quoted := make([]string, len(labels))
	for i, l := range labels {
		quoted[i] = regexp.QuoteMeta(l)
	}
	return regexp.MustCompile(`(?m)^(` + strings.Join(quoted, "|") + `):`)
}

// extractByLabelPrefixes harvests `LABEL: value` sections from reasoning,
// one per provided label. Used when an agent has opted in to the
// label-prefix harvest flag (Phase 6 Strategy 4) and the provider has
// populated FormatState.Labels from the system prompt schema.
//
// Matching rules:
//   - Labels are matched line-anchored (^LABEL:), case-sensitive.
//   - Each value extends from the end of "LABEL:" to the start of the
//     next label-occurrence anywhere in the text, or end of text.
//   - The LAST occurrence of each label wins — the model often retries or
//     refines its enumeration, and the latest version is the one to keep.
//
// Confidence guard: require at least half the provided labels to be found
// (or all when ≤ 3 labels). Below threshold returns ("", false) and the
// caller falls through to subsequent strategies.
func extractByLabelPrefixes(reasoning string, labels []string) (string, bool) {
	if len(labels) < 1 {
		return "", false
	}

	re := labelPrefixRe(labels)
	matches := re.FindAllStringSubmatchIndex(reasoning, -1)
	if len(matches) == 0 {
		return "", false
	}

	type span struct {
		valueStart, valueEnd int
	}
	last := make(map[string]span, len(labels))
	for i, m := range matches {
		label := reasoning[m[2]:m[3]]
		valueStart := m[1] // position right after the colon
		var valueEnd int
		if i+1 < len(matches) {
			valueEnd = matches[i+1][0]
		} else {
			valueEnd = len(reasoning)
		}
		last[label] = span{valueStart: valueStart, valueEnd: valueEnd}
	}

	minRequired := (len(labels) + 1) / 2
	if len(labels) <= 3 {
		minRequired = len(labels)
	}
	if len(last) < minRequired {
		return "", false
	}

	var b strings.Builder
	for _, l := range labels {
		s, ok := last[l]
		if !ok {
			continue
		}
		val := strings.TrimSpace(reasoning[s.valueStart:s.valueEnd])
		if val == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(l)
		b.WriteString(": ")
		b.WriteString(val)
	}

	out := b.String()
	if out == "" {
		return "", false
	}
	return out, true
}

// scanPromptLabels extracts ALL_CAPS_LABEL: candidates from a prompt string.
// Convenience helper for providers that opt agents in to label-prefix
// harvest: scan the system prompt for the agent's output schema and pass
// the result to FormatState.Labels before the request runs.
//
// Match rule: line-anchored, ASCII upper-case + digits + underscore, length
// ≥ 3, followed by a colon. Returns labels in first-occurrence order,
// deduplicated.
//
// Currently unused by the format-package itself; exposed for providers that
// handle the agent-config → state.Labels plumbing in a follow-up PR.
func scanPromptLabels(prompt string) []string {
	re := regexp.MustCompile(`(?m)^([A-Z][A-Z0-9_]{2,}):`)
	matches := re.FindAllStringSubmatch(prompt, -1)
	seen := map[string]bool{}
	var labels []string
	for _, m := range matches {
		if !seen[m[1]] {
			seen[m[1]] = true
			labels = append(labels, m[1])
		}
	}
	return labels
}

// extractTrailingStructuredList finds the last qualifying contiguous
// bullet/numbered list whose end is in the trailing half of the reasoning
// text and returns the list's text.
//
// Heuristic motivation: when reasoning models emit the answer as an
// enumeration (merged PRs, file enumerations, step-by-step results) WITHOUT
// surrounding the list with a markdown header or "Final answer:" marker, the
// existing strategies miss it entirely. The list itself IS the answer.
//
// To avoid stealing tool-output quotes (the model echoes a `git log` listing
// in the middle of CoT, then thinks about it, then emits its own answer
// elsewhere), the candidate block must:
//
//  1. Contain ≥ 5 contiguous list items (single-blank-line gaps allowed).
//  2. END in the trailing 50% of the reasoning text (by line index) — so a
//     list buried in the first half is treated as a tool-output quote, not
//     the answer.
//  3. Be the LAST qualifying block — picking the latest position favors the
//     model's own committed enumeration over earlier tool-echoes, even when
//     the earlier echo has more items.
//
// A list item is a line whose leading-whitespace-trimmed prefix is "- ",
// "* ", "+ ", or `\d+\. `. Indented sub-items are accepted; nested-list
// structure is preserved.
func extractTrailingStructuredList(reasoning string) (string, bool) {
	lines := strings.Split(reasoning, "\n")
	if len(lines) < 5 {
		return "", false
	}

	type block struct {
		start, end int // inclusive line indices
		items      int
	}
	var blocks []block
	var cur *block
	for i, line := range lines {
		if isStructuredListItem(line) {
			if cur == nil {
				cur = &block{start: i, end: i, items: 1}
			} else {
				cur.end = i
				cur.items++
			}
			continue
		}
		// Allow a single blank line between list items (don't close the block).
		if cur != nil && strings.TrimSpace(line) == "" && i+1 < len(lines) && isStructuredListItem(lines[i+1]) {
			continue
		}
		if cur != nil {
			blocks = append(blocks, *cur)
			cur = nil
		}
	}
	if cur != nil {
		blocks = append(blocks, *cur)
	}

	trailingHalfStart := len(lines) / 2 // block.end must be >= this

	// Walk blocks in reverse to find the LAST qualifying one.
	for i := len(blocks) - 1; i >= 0; i-- {
		b := blocks[i]
		if b.items < 5 {
			continue
		}
		if b.end < trailingHalfStart {
			continue
		}
		candidate := strings.Join(lines[b.start:b.end+1], "\n")
		return strings.TrimSpace(candidate), true
	}

	return "", false
}

// isStructuredListItem reports whether a line is a markdown list item:
// "- ", "* ", "+ ", or "<digits>. ". Leading whitespace (indentation) is
// allowed and stripped before the prefix check, so nested items qualify.
func isStructuredListItem(line string) bool {
	t := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "+ ") {
		return true
	}
	// Numbered list: \d+\.<space>
	if len(t) >= 3 && t[0] >= '0' && t[0] <= '9' {
		i := 1
		for i < len(t) && t[i] >= '0' && t[i] <= '9' {
			i++
		}
		if i+1 < len(t) && t[i] == '.' && t[i+1] == ' ' {
			return true
		}
	}
	return false
}

// isMarkdownHeaderLine reports whether a line is a top-level markdown header
// (# / ## / ###), ignoring leading whitespace.
func isMarkdownHeaderLine(line string) bool {
	t := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(t, "# ") || strings.HasPrefix(t, "## ") || strings.HasPrefix(t, "### ")
}

// findTrailingMarkdownReport returns the byte index of the start of a trailing
// markdown report block, or -1 if none found.
//
// Heuristic: start from the LAST markdown header (# / ## / ###) in the text —
// that's the start of the final synthesis, not a quote of a source the model
// was reading — then walk earlier headers backward, merging each one into the
// report as long as only blanks/list-items/other headers separate it from the
// report found so far. A stretch of ordinary prose between two headers means
// the earlier header belongs to a different section (e.g. reasoning quoting a
// tool-result header), not the model's own trailing report, so the merge
// stops there. This lets a genuine multi-section report ("## This week" /
// "## Gate status" back to back) extract in full while still rejecting an
// unrelated earlier header.
//
// Once the report's start is settled, return its position if the block from
// there to the end is structurally substantial enough to look like a real
// report — at least 2 header lines OR at least 3 non-empty content lines in
// the whole tail. The goal is to separate "model was thinking, then wrote the
// report" from "model was thinking with a one-off header reference".
func findTrailingMarkdownReport(s string) int {
	lines := strings.Split(s, "\n")

	var headers []int
	charOffset := 0
	offsets := make([]int, len(lines))
	for i, line := range lines {
		offsets[i] = charOffset
		if isMarkdownHeaderLine(line) {
			headers = append(headers, i)
		}
		charOffset += len(line) + 1 // +1 for \n
	}
	if len(headers) == 0 {
		return -1
	}

	reportStart := headers[len(headers)-1]
	for k := len(headers) - 2; k >= 0; k-- {
		clean := true
		for i := headers[k] + 1; i < reportStart; i++ {
			if strings.TrimSpace(lines[i]) == "" || isStructuredListItem(lines[i]) || isMarkdownHeaderLine(lines[i]) {
				continue
			}
			clean = false
			break
		}
		if !clean {
			break
		}
		reportStart = headers[k]
	}

	// Validate the tail block has enough structure to be called a report.
	headerCount := 0
	nonEmptyCount := 0
	for i := reportStart; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		nonEmptyCount++
		if isMarkdownHeaderLine(lines[i]) {
			headerCount++
		}
	}
	if headerCount < 2 && nonEmptyCount < 3 {
		return -1
	}
	return offsets[reportStart]
}
