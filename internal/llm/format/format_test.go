package format

import (
	"strings"
	"testing"
)

// ---------- Detection / Registry ----------

// chainContains reports whether a wrapping chain contains an adapter with the given name.
// Used in Detect tests to check that a model routes to a chain that INCLUDES
// a target base adapter, regardless of which wrappers sit on top of it.
func chainContains(a ResponseFormat, name string) bool {
	for _, n := range ChainNames(a) {
		if n == name {
			return true
		}
	}
	return false
}

func TestDetect_NemotronMatchesReasoningAdapter(t *testing.T) {
	a := Detect("vllm-nemotron-elastic-30b", "")
	if !chainContains(a, "reasoning_content_field") {
		t.Errorf("nemotron chain should contain reasoning_content_field, got chain %v", ChainNames(a))
	}
}

func TestDetect_DeepSeekR1MatchesReasoningAdapter(t *testing.T) {
	for _, m := range []string{"deepseek-r1", "deepseek-r1-distill-qwen-32b", "deepseek-reasoner"} {
		a := Detect(m, "")
		if !chainContains(a, "reasoning_content_field") {
			t.Errorf("%s chain should contain reasoning_content_field, got chain %v", m, ChainNames(a))
		}
	}
}

func TestDetect_NemotronChainIncludesInlineFallbacks(t *testing.T) {
	// Nemotron is wrapped with ThinkTagInline + ToolCallTagInline as a
	// defensive safety net. Verify the wrappers are actually in place so
	// we don't accidentally regress Phase 2's "chain ordering matters" guarantee.
	a := Detect("vllm-nemotron-elastic-30b", "")
	chain := ChainNames(a)
	expected := []string{"reasoning_content_field", "think_tag_inline", "tool_call_tag_inline"}
	if len(chain) != len(expected) {
		t.Fatalf("expected chain %v, got %v", expected, chain)
	}
	for i, n := range expected {
		if chain[i] != n {
			t.Errorf("chain[%d] = %s, want %s", i, chain[i], n)
		}
	}
}

func TestDetect_SniffingForUnknownModel(t *testing.T) {
	// Unknown models fall back to Sniffing (runtime auto-detection) so new
	// reasoning model families don't silently drop reasoning_content.
	a := Detect("gpt-4o-mini", "")
	if a.Name() != "sniffing" {
		t.Errorf("unknown model should fall back to sniffing, got %s", a.Name())
	}
}

func TestDetect_ExplicitStandardOpenAIOverride(t *testing.T) {
	// Users who know their model is a plain OpenAI model can opt out of
	// sniffing entirely via the explicit override.
	a := Detect("my-custom-model", "standard_openai")
	if a.Name() != "standard_openai" {
		t.Errorf("explicit standard_openai override should work, got %s", a.Name())
	}
}

func TestDetect_ExplicitOverrideWins(t *testing.T) {
	// nemotron would normally route to reasoning_content_field; explicit override forces standard.
	a := Detect("vllm-nemotron-elastic-30b", "standard_openai")
	if a.Name() != "standard_openai" {
		t.Errorf("explicit override should win, got %s", a.Name())
	}
}

func TestDetect_UnknownOverrideFallsThroughToPattern(t *testing.T) {
	// An unknown override name should NOT silently break; the model-pattern
	// detection still runs and catches nemotron. The nemotron chain contains
	// reasoning_content_field at its core.
	a := Detect("vllm-nemotron-elastic-30b", "nonexistent_adapter")
	if !chainContains(a, "reasoning_content_field") {
		t.Errorf("unknown override should fall through to pattern match containing reasoning_content_field, got %v", ChainNames(a))
	}
}

// ---------- StandardOpenAI adapter ----------

func TestStandardOpenAI_StreamingContentOnly(t *testing.T) {
	a := StandardOpenAI{}
	state := NewState()
	a.ApplyDelta(state, RawDelta{Content: "Hello "})
	a.ApplyDelta(state, RawDelta{Content: "world"})
	a.ApplyDelta(state, RawDelta{FinishReason: "stop"})

	r := a.Finalize(state)
	if r.Content != "Hello world" {
		t.Errorf("content = %q, want %q", r.Content, "Hello world")
	}
	if r.Reasoning != "" {
		t.Errorf("reasoning should be empty, got %q", r.Reasoning)
	}
	if r.FinishReason != "stop" {
		t.Errorf("finish = %q, want stop", r.FinishReason)
	}
}

func TestStandardOpenAI_StreamingToolCall(t *testing.T) {
	a := StandardOpenAI{}
	state := NewState()
	a.ApplyDelta(state, RawDelta{
		ToolCalls: []RawDeltaToolCall{
			{Index: 0, ID: "call_1", Name: "search", Arguments: `{"q":"`},
		},
	})
	a.ApplyDelta(state, RawDelta{
		ToolCalls: []RawDeltaToolCall{
			{Index: 0, Arguments: `test"}`},
		},
	})
	a.ApplyDelta(state, RawDelta{FinishReason: "tool_calls"})

	r := a.Finalize(state)
	if len(r.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(r.ToolCalls))
	}
	tc := r.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "search" {
		t.Errorf("tool call id/name = %q/%q", tc.ID, tc.Name)
	}
	if tc.Arguments["q"] != "test" {
		t.Errorf("tool call args = %v", tc.Arguments)
	}
}

// TestStandardOpenAI_MultipleToolCallsPreserveIssueOrder is a regression
// test: ToolMeta is a map keyed by index, so ranging over it without sorting
// produced a nondeterministic ToolCalls order run to run.
func TestStandardOpenAI_MultipleToolCallsPreserveIssueOrder(t *testing.T) {
	a := StandardOpenAI{}
	for attempt := 0; attempt < 20; attempt++ {
		state := NewState()
		a.ApplyDelta(state, RawDelta{
			ToolCalls: []RawDeltaToolCall{{Index: 0, ID: "call_0", Name: "first", Arguments: `{}`}},
		})
		a.ApplyDelta(state, RawDelta{
			ToolCalls: []RawDeltaToolCall{{Index: 1, ID: "call_1", Name: "second", Arguments: `{}`}},
		})
		a.ApplyDelta(state, RawDelta{
			ToolCalls: []RawDeltaToolCall{{Index: 2, ID: "call_2", Name: "third", Arguments: `{}`}},
		})
		a.ApplyDelta(state, RawDelta{FinishReason: "tool_calls"})

		r := a.Finalize(state)
		if len(r.ToolCalls) != 3 {
			t.Fatalf("attempt %d: expected 3 tool calls, got %d", attempt, len(r.ToolCalls))
		}
		wantIDs := []string{"call_0", "call_1", "call_2"}
		for i, want := range wantIDs {
			if r.ToolCalls[i].ID != want {
				t.Fatalf("attempt %d: ToolCalls[%d].ID = %q, want %q (order not preserved)", attempt, i, r.ToolCalls[i].ID, want)
			}
		}
	}
}

// ---------- ReasoningContentField adapter ----------

func TestReasoningContentField_StreamingReasoningPromotedWhenContentEmpty(t *testing.T) {
	// Nemotron-style: all output on reasoning_content, content empty, reasoning
	// ends with a clear final-answer marker.
	a := ReasoningContentField{}
	state := NewState()
	a.ApplyDelta(state, RawDelta{ReasoningContent: "We need to count things. "})
	a.ApplyDelta(state, RawDelta{ReasoningContent: "Let's see... There are 5 items. "})
	a.ApplyDelta(state, RawDelta{ReasoningContent: "Final answer: The count is 5 items across all categories."})
	a.ApplyDelta(state, RawDelta{FinishReason: "length"})

	r := a.Finalize(state)
	if !strings.Contains(r.Content, "The count is 5 items across all categories") {
		t.Errorf("content should contain extracted final answer, got: %q", r.Content)
	}
	if strings.HasPrefix(r.Content, reasoningOnlyMarker) {
		t.Error("extracted answer should NOT have the reasoning-only marker prefix")
	}
	if r.Reasoning == "" {
		t.Error("reasoning field should still be populated")
	}
}

func TestReasoningContentField_ReasoningOnlyFallsBackToMarker(t *testing.T) {
	// Reasoning has no clear final-answer marker → should promote full reasoning with warning.
	a := ReasoningContentField{}
	state := NewState()
	a.ApplyDelta(state, RawDelta{ReasoningContent: "We need to think about this. "})
	a.ApplyDelta(state, RawDelta{ReasoningContent: "Hmm, let me ponder. "})
	a.ApplyDelta(state, RawDelta{FinishReason: "length"})

	r := a.Finalize(state)
	if !strings.HasPrefix(r.Content, reasoningOnlyMarker) {
		t.Errorf("unmarked reasoning should get the warning marker prefix, got: %q", r.Content[:min(100, len(r.Content))])
	}
	if !strings.Contains(r.Content, "ponder") {
		t.Error("reasoning content should still appear after the marker")
	}
	if !r.Salvaged {
		t.Error("reasoning-only fallback should set Result.Salvaged=true (Phase 6.2 Mitigation A)")
	}
}

func TestReasoningContentField_ExtractedAnswerNotSalvaged(t *testing.T) {
	// When extraction succeeds (Strategy 1 marker matches), the result is a
	// real commitment — Salvaged MUST stay false.
	a := ReasoningContentField{}
	state := NewState()
	a.ApplyDelta(state, RawDelta{ReasoningContent: "Working through the steps. Final answer: The system handles 1024 concurrent connections."})
	a.ApplyDelta(state, RawDelta{FinishReason: "length"})

	r := a.Finalize(state)
	if strings.HasPrefix(r.Content, reasoningOnlyMarker) {
		t.Errorf("extracted answer should NOT have the marker prefix, got: %q", r.Content)
	}
	if r.Salvaged {
		t.Error("extracted answer should NOT be marked Salvaged")
	}
}

func TestReasoningContentField_ContentPathNotSalvaged(t *testing.T) {
	// When the model emits real content (not via fallback), Salvaged must stay false.
	a := ReasoningContentField{}
	state := NewState()
	a.ApplyDelta(state, RawDelta{Content: "Plain content answer."})
	a.ApplyDelta(state, RawDelta{FinishReason: "stop"})

	r := a.Finalize(state)
	if r.Salvaged {
		t.Error("content-path result should NOT be marked Salvaged")
	}
}

func TestReasoningContentField_ToolCallPathNotSalvaged(t *testing.T) {
	// When the model emits a tool call, we never enter the salvage path.
	a := ReasoningContentField{}
	state := NewState()
	a.ApplyDelta(state, RawDelta{ReasoningContent: "Let me call the search tool."})
	a.ApplyDelta(state, RawDelta{
		ToolCalls: []RawDeltaToolCall{
			{Index: 0, ID: "c1", Name: "search", Arguments: `{"q":"x"}`},
		},
	})

	r := a.Finalize(state)
	if r.Salvaged {
		t.Error("tool-call result should NOT be marked Salvaged")
	}
}

func TestReasoningContentField_ContentPresentTakesPrecedence(t *testing.T) {
	// When both content and reasoning are present, content should be used as-is
	// and reasoning should NOT trigger the marker.
	a := ReasoningContentField{}
	state := NewState()
	a.ApplyDelta(state, RawDelta{ReasoningContent: "thinking... "})
	a.ApplyDelta(state, RawDelta{Content: "The answer is 42."})
	a.ApplyDelta(state, RawDelta{FinishReason: "stop"})

	r := a.Finalize(state)
	if r.Content != "The answer is 42." {
		t.Errorf("content should be the real content, got %q", r.Content)
	}
	if strings.Contains(r.Content, reasoningOnlyMarker) {
		t.Error("marker should NOT appear when content is present")
	}
	if r.Reasoning != "thinking... " {
		t.Errorf("reasoning should still be captured, got %q", r.Reasoning)
	}
}

func TestReasoningContentField_ToolCallsPreventReasoningPromotion(t *testing.T) {
	// If the model emits a tool call (even with reasoning and no content),
	// we should NOT try to extract a "final answer" — the answer is the tool call.
	a := ReasoningContentField{}
	state := NewState()
	a.ApplyDelta(state, RawDelta{ReasoningContent: "I should call the search tool."})
	a.ApplyDelta(state, RawDelta{
		ToolCalls: []RawDeltaToolCall{
			{Index: 0, ID: "c1", Name: "search", Arguments: `{"q":"x"}`},
		},
	})

	r := a.Finalize(state)
	if r.Content != "" {
		t.Errorf("content should be empty when tool calls present, got %q", r.Content)
	}
	if len(r.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(r.ToolCalls))
	}
}

func TestReasoningContentField_NonStreamingReasoningExtraction(t *testing.T) {
	a := ReasoningContentField{}
	state := NewState()
	a.ApplyFull(state, RawMessage{
		Content:          "",
		ReasoningContent: "Let me think carefully. Thus: The answer is 42 across all known dimensions.",
		FinishReason:     "length",
	})
	r := a.Finalize(state)
	if !strings.Contains(r.Content, "42 across all known dimensions") {
		t.Errorf("non-streaming path should extract final answer, got %q", r.Content)
	}
}

// ---------- extractFinalAnswer direct tests ----------

func TestExtractFinalAnswer_ExplicitMarker(t *testing.T) {
	cases := []struct {
		reasoning string
		wantHit   bool
	}{
		{"We thought about it. Final answer: The result is 42 plus or minus one.", true},
		{"Thinking step. Thus, the output is a comprehensive report with 3 sections.", true},
		{"So the final answer is: Comprehensive summary of all the work done today.", true},
		{"Here is the final report: A complete analysis of the system.", true},
		// Too-short extraction should be rejected.
		{"Thus: ok", false},
		// No marker at all.
		{"Just some thinking with no conclusion.", false},
	}
	for _, c := range cases {
		_, ok := extractFinalAnswer(c.reasoning)
		if ok != c.wantHit {
			t.Errorf("extractFinalAnswer(%q) hit=%v, want %v", c.reasoning, ok, c.wantHit)
		}
	}
}

func TestExtractFinalAnswer_StrongMarkerWinsOverEarlierWeakOne(t *testing.T) {
	// Regression: finalAnswerMarkers used to be one combined alternation
	// regex. Go's regexp finds the leftmost match *position*, so a weak
	// marker ("thus,") appearing earlier in the text pre-empted a stronger
	// one ("final answer:") appearing later, and its greedy capture
	// swallowed everything through the real marker — extracting
	// "that seems right. Final answer: X is correct..." instead of just
	// "X is correct...".
	reasoning := "Thus, that seems right. Final answer: X is correct and complete here."
	got, ok := extractFinalAnswer(reasoning)
	if !ok {
		t.Fatalf("extractFinalAnswer(%q) = false, want true", reasoning)
	}
	if strings.Contains(got, "Final answer") || strings.HasPrefix(got, "that seems right") {
		t.Errorf("extractFinalAnswer(%q) = %q, want it to start at the stronger marker, not swallow the weaker one's lead-in", reasoning, got)
	}
	if !strings.HasPrefix(got, "X is correct") {
		t.Errorf("extractFinalAnswer(%q) = %q, want it to start with %q", reasoning, got, "X is correct")
	}
}

func TestExtractFinalAnswer_TrailingMarkdownReport(t *testing.T) {
	reasoning := `We need to produce a report. Let me think about structure.
First I'll list the sections, then fill them in.

## This week
- Merged PR #9 fixing the streaming bug
- Added test coverage

## Gate status
- Current gate is Gate 1`
	extracted, ok := extractFinalAnswer(reasoning)
	if !ok {
		t.Fatal("expected to extract trailing markdown report")
	}
	if !strings.Contains(extracted, "## This week") {
		t.Errorf("extraction should include the markdown headers, got: %q", extracted)
	}
	if strings.Contains(extracted, "We need to produce") {
		t.Errorf("extraction should NOT include the pre-report thinking, got: %q", extracted)
	}
}

// TestExtractFinalAnswer_RejectsEarlierUnrelatedHeader is a regression test
// for the reviewer's original scenario: an earlier header belongs to a
// different section (e.g. reasoning quoting a tool-result header), separated
// from the real trailing report by ordinary prose. Only the trailing report
// must be extracted.
func TestExtractFinalAnswer_RejectsEarlierUnrelatedHeader(t *testing.T) {
	reasoning := `## Plan
I should check the git log for recent merges before answering.

Looking at the tool output, I see several commits. Let me think about
how to summarize this for the user in a clear way.

## This week
- Merged PR #9 fixing the streaming bug
- Added test coverage`
	extracted, ok := extractFinalAnswer(reasoning)
	if !ok {
		t.Fatal("expected to extract trailing markdown report")
	}
	if !strings.Contains(extracted, "## This week") {
		t.Errorf("extraction should include the real trailing report, got: %q", extracted)
	}
	if strings.Contains(extracted, "## Plan") {
		t.Errorf("extraction should NOT include the earlier unrelated header, got: %q", extracted)
	}
}

func TestExtractFinalAnswer_EmptyInput(t *testing.T) {
	if _, ok := extractFinalAnswer(""); ok {
		t.Error("empty input should not extract")
	}
	if _, ok := extractFinalAnswer("   \n  "); ok {
		t.Error("whitespace-only input should not extract")
	}
}

// ---------- Strategy 5: loose imperative marker ----------

func TestExtractFinalAnswer_Strategy5_LooseImperativeMarker_ShortConclusion(t *testing.T) {
	cases := []struct {
		name      string
		reasoning string
		wantHit   bool
		wantSub   string
	}{
		{
			name:      "thus + we + have committed conclusion",
			reasoning: "Looking at the listing, the file does exist. Thus we have determined that the file exists at internal/llm/format/format.go.",
			wantHit:   true,
			wantSub:   "the file exists at internal/llm/format/format.go",
		},
		{
			name:      "therefore + the answer + becomes",
			reasoning: "After reviewing all the evidence carefully and weighing all the options. Therefore the answer becomes that the system handles 1024 concurrent connections.",
			wantHit:   true,
			wantSub:   "the system handles 1024 concurrent connections",
		},
		{
			name:      "so + I + will + concrete answer",
			reasoning: "I have computed the totals from each of the input rows. So I will return the final sum of 12345 across all categories.",
			wantHit:   true,
			wantSub:   "12345 across all categories",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			extracted, ok := extractFinalAnswer(c.reasoning)
			if ok != c.wantHit {
				t.Fatalf("hit=%v want %v; extracted=%q", ok, c.wantHit, extracted)
			}
			if c.wantHit && !strings.Contains(extracted, c.wantSub) {
				t.Errorf("extracted=%q should contain %q", extracted, c.wantSub)
			}
		})
	}
}

func TestExtractFinalAnswer_Strategy5_RejectsMetaCommentary(t *testing.T) {
	// These are exactly the phrasings Strategy 5's regex would match if it
	// were unguarded — but looksLikeOutput must reject them because the
	// captured text is more deliberation, not a committed answer.
	rejectCases := []string{
		// The WeeklyStatusReport-style failure: model is talking ABOUT the
		// answer it was supposed to produce, not committing to one.
		"Looking at the merged PRs in the log. Thus we need to output these lines after the MERGED_PRS label.",
		// Self-correction mid-stream.
		"After thinking about the logic carefully. So we will need to reconsider the approach because the first attempt was wrong.",
		// "Let me" planning phrase.
		"Therefore i have to find the answer. Let me think about how to express it cleanly first.",
	}
	for i, r := range rejectCases {
		extracted, ok := extractFinalAnswer(r)
		if ok {
			t.Errorf("case %d should NOT extract (meta-commentary), but got %q", i, extracted)
		}
	}
}

func TestExtractFinalAnswer_Strategy5_OrderedAfterStrategies1And2(t *testing.T) {
	// When Strategy 1 (strict marker) matches, Strategy 5 must not steal.
	r1 := "We thought about it. Final answer: The result is 42 with confidence high. Thus we must commit this."
	got1, ok1 := extractFinalAnswer(r1)
	if !ok1 {
		t.Fatal("strategy 1 should match")
	}
	if !strings.HasPrefix(got1, "The result is 42") {
		t.Errorf("expected Strategy 1 win starting with 'The result is 42', got %q", got1)
	}

	// When Strategy 2 (trailing markdown) matches, Strategy 5 must not steal.
	r2 := "Background thinking. Thus we have produced a report.\n\n## Findings\n- Finding A\n- Finding B\n## Conclusion\n- Done"
	got2, ok2 := extractFinalAnswer(r2)
	if !ok2 {
		t.Fatal("strategy 2 should match")
	}
	if !strings.Contains(got2, "## Findings") {
		t.Errorf("expected Strategy 2 (markdown) win, got %q", got2)
	}
}

// ---------- looksLikeOutput unit tests ----------

func TestLooksLikeOutput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"whitespace", "   \n  ", false},
		{"short clean conclusion", "the file exists at path/to/file.go.", true},
		{"meta we need", "we need to figure this out before answering.", false},
		{"meta let me", "let me think about this more carefully now.", false},
		{"meta but wait", "the answer is 5. but wait, that might be wrong.", false},
		{"long with bullets", strings.Repeat("filler text. ", 30) + "\n- item one\n- item two\n- item three", true},
		{"long with single header", strings.Repeat("filler text. ", 30) + "\n## Section\nbody body body body body", true},
		{"long with code fence", strings.Repeat("filler text. ", 30) + "\n```\ncode here\n```", true},
		{"long unstructured prose", strings.Repeat("filler text without any markers anywhere here just words. ", 6), false},
		{"numbered list", strings.Repeat("filler text. ", 30) + "\n1. step one\n2. step two\n3. step three", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := looksLikeOutput(c.in)
			if got != c.want {
				t.Errorf("looksLikeOutput(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------- Strategy 3: trailing structured-list ----------

func TestExtractFinalAnswer_Strategy3_TrailingBulletList_WeeklyStatusShape(t *testing.T) {
	// Hand-crafted approximation of the captured WeeklyStatusReport failure
	// shape (~/.rakitsu/sessions/05f15af9-…). The reasoning enumerates merged
	// PRs in a trailing bullet block, then drifts into meta-commentary before
	// being truncated by max_tokens.
	reasoning := `We need to gather the merged PRs for the weekly status report.
Looking at the git log output, I can see all the merge commits.
Let me enumerate each one and format it for the MERGED_PRS section.
The instruction says to output a bullet per merged PR with the SHA and title.

I'll start from the top of the log and work down. Each line should be
formatted as "- <sha> <title>". Let me write them out now.

The merged PRs from the past week:

- 0000000 Merge pull request #NN from org/repo/feat/feature-a
- 0000001 Merge pull request #NN from org/repo/fix/bug-b
- 0000002 Merge pull request #NN from org/repo/chore/cleanup-c
- 0000003 Merge pull request #NN from org/repo/docs/doc-d
- 0000004 Merge pull request #NN from org/repo/refactor/refactor-e
- 0000005 Merge pull request #NN from org/repo/test/test-f
- 0000006 Merge pull request #NN from org/repo/perf/perf-g

Thus we need to output these lines after "MERGED_PRS:". That's many lines, but
it's okay. Now OPEN_BUGS_SAMPLE`

	extracted, ok := extractFinalAnswer(reasoning)
	if !ok {
		t.Fatal("expected trailing bullet list to extract")
	}
	if !strings.Contains(extracted, "- 0000000 Merge pull request") {
		t.Errorf("extraction should include first bullet, got: %q", extracted)
	}
	if !strings.Contains(extracted, "- 0000006 Merge pull request") {
		t.Errorf("extraction should include last bullet, got: %q", extracted)
	}
	if strings.Contains(extracted, "We need to gather") {
		t.Errorf("extraction should NOT include preamble CoT, got: %q", extracted)
	}
	if strings.Contains(extracted, "Thus we need to output") {
		t.Errorf("extraction should NOT include trailing meta-commentary, got: %q", extracted)
	}
}

func TestExtractFinalAnswer_Strategy3_MidTextListRejected(t *testing.T) {
	// A bullet list in the FIRST half of reasoning (e.g., model echoed a
	// tool result like `ls` output, then continued thinking) should NOT be
	// promoted as the answer — the model didn't commit to it.
	reasoning := `Looking at the directory listing from the tool call:

- file_a.go
- file_b.go
- file_c.go
- file_d.go
- file_e.go
- file_f.go

After examining each one, I think we need to consider the structure more
carefully before producing a recommendation. Let me think about what the
user is actually asking for. They want a summary of the most important
files. Hmm, that's tricky to decide. I should probably check the
imports too. Let me look at more context first.

Maybe I should focus on the entry points and the main packages. That
seems like the most useful direction to take. Continuing to think about
how to structure the answer in a useful way for the operator. I'll
need to consider several factors here including the overall architecture
and how the different pieces fit together.`
	if extracted, ok := extractFinalAnswer(reasoning); ok {
		t.Errorf("mid-text bullet list (not in trailing 30%%) should NOT extract, got: %q", extracted)
	}
}

func TestExtractFinalAnswer_Strategy3_ShortListRejected(t *testing.T) {
	// A trailing list with fewer than 5 items should NOT extract — the
	// minimum-items threshold protects against random "I have:\n- one\n- two"
	// patterns being treated as the answer.
	reasoning := `We need to count things from the input. Let me see what we have.
Looking at the data, I observe several items.
The set contains:

- one
- two
- three`
	if extracted, ok := extractFinalAnswer(reasoning); ok {
		t.Errorf("short trailing list (< 5 items) should NOT extract, got: %q", extracted)
	}
}

func TestExtractFinalAnswer_Strategy3_NumberedListExtracts(t *testing.T) {
	// Numbered lists ("1. ", "2. ", …) should extract just like bullet lists.
	reasoning := `The user asked for a step-by-step plan. I'll lay it out
in numbered order so each step is easy to track. Let me think about
the dependency order first to avoid ordering issues. I want each step
to be actionable and concrete so the operator can execute them in
sequence without ambiguity. That means each step should have a clear
verb and object. Let me write them out now in order.

The plan:

1. Validate the input config against the schema.
2. Resolve provider references to concrete connections.
3. Initialize the tool registry with the configured tools.
4. Build the agent dependency graph from the orchestrator.
5. Start the SSE hub and wait for run requests.
6. Dispatch each run to its agent and stream events back.`
	extracted, ok := extractFinalAnswer(reasoning)
	if !ok {
		t.Fatal("expected numbered list to extract")
	}
	if !strings.Contains(extracted, "1. Validate") {
		t.Errorf("should include first numbered item, got: %q", extracted)
	}
	if !strings.Contains(extracted, "6. Dispatch") {
		t.Errorf("should include last numbered item, got: %q", extracted)
	}
}

func TestExtractFinalAnswer_Strategy3_OrderedAfterStrategies1And2(t *testing.T) {
	// Strategy 1 win: explicit marker beats trailing list.
	r1 := `Looking at the merged PRs in the log, I'll enumerate them.

Final answer: The merged PRs for the past week are listed below in chronological order.

- 0000000 PR title one
- 0000001 PR title two
- 0000002 PR title three
- 0000003 PR title four
- 0000004 PR title five
- 0000005 PR title six`
	got1, ok1 := extractFinalAnswer(r1)
	if !ok1 {
		t.Fatal("strategy 1 should win")
	}
	if !strings.HasPrefix(got1, "The merged PRs for the past week") {
		t.Errorf("expected Strategy 1 win, got: %q", got1)
	}

	// Strategy 2 win: trailing markdown report with bullets beats bare list.
	r2 := `Background thinking about the structure.

## Merged PRs This Week
- 0000000 PR a
- 0000001 PR b
- 0000002 PR c
- 0000003 PR d
- 0000004 PR e
- 0000005 PR f`
	got2, ok2 := extractFinalAnswer(r2)
	if !ok2 {
		t.Fatal("strategy 2 should win")
	}
	if !strings.Contains(got2, "## Merged PRs This Week") {
		t.Errorf("expected Strategy 2 win including the header, got: %q", got2)
	}
}

func TestExtractTrailingStructuredList_HandlesBlankLineGaps(t *testing.T) {
	// A single blank line between items should NOT break the contiguous run.
	reasoning := `We need to enumerate the items here.
Let me write them out.

The items:

- item one

- item two

- item three

- item four

- item five`
	extracted, ok := extractTrailingStructuredList(reasoning)
	if !ok {
		t.Fatal("expected extraction across blank-line gaps")
	}
	for _, want := range []string{"- item one", "- item three", "- item five"} {
		if !strings.Contains(extracted, want) {
			t.Errorf("extracted should contain %q, got: %q", want, extracted)
		}
	}
}

// ---------- Strategy 4: label-prefix harvest ----------

func TestExtractFinalAnswerWithLabels_Strategy4_StockAnalysisShape(t *testing.T) {
	// Captured shape: CSV-interpretation prompt declares output labels in the
	// system prompt; reasoning model enumerates each label inline in CoT
	// framing. Strategy 4 harvests each label's last-occurrence value.
	labels := []string{"COLUMNS", "FIRST_DATE", "LAST_DATE", "ROW_COUNT"}
	reasoning := `We need to parse the CSV to extract the schema fields.
Looking at the header row of the CSV file content we already loaded.

The header is: date,open,high,low,close,volume,sma20,sma50,rsi14

I need to figure out the date range too. Let me look at the first and
last data rows. The first row has date 2024-01-02 and the last row has
date 2024-12-31. The total row count from the listing is 252.

Now let me write the output schema fields:

COLUMNS: date, open, high, low, close, volume, sma20, sma50, rsi14
FIRST_DATE: 2024-01-02
LAST_DATE: 2024-12-31
ROW_COUNT: 252

Thus we have all four output fields populated.`

	extracted, ok := extractFinalAnswerWithLabels(reasoning, labels)
	if !ok {
		t.Fatal("expected Strategy 4 to harvest labels")
	}
	for _, want := range []string{
		"COLUMNS: date, open, high, low, close, volume, sma20, sma50, rsi14",
		"FIRST_DATE: 2024-01-02",
		"LAST_DATE: 2024-12-31",
		"ROW_COUNT: 252",
	} {
		if !strings.Contains(extracted, want) {
			t.Errorf("extracted should contain %q\nGot:\n%s", want, extracted)
		}
	}
}

func TestExtractFinalAnswerWithLabels_Strategy4_LastOccurrenceWins(t *testing.T) {
	labels := []string{"ANSWER_A", "ANSWER_B"}
	reasoning := `Initial draft:

ANSWER_A: first draft value
ANSWER_B: first draft other value

Wait, let me reconsider. The values should actually be:

ANSWER_A: refined final value
ANSWER_B: refined final other value`

	extracted, ok := extractFinalAnswerWithLabels(reasoning, labels)
	if !ok {
		t.Fatal("expected Strategy 4 to harvest refined values")
	}
	if !strings.Contains(extracted, "refined final value") {
		t.Errorf("should pick refined ANSWER_A value\nGot:\n%s", extracted)
	}
	if !strings.Contains(extracted, "refined final other value") {
		t.Errorf("should pick refined ANSWER_B value\nGot:\n%s", extracted)
	}
	if strings.Contains(extracted, "first draft") {
		t.Errorf("should NOT include first-draft text\nGot:\n%s", extracted)
	}
}

func TestExtractFinalAnswerWithLabels_Strategy4_BelowThresholdFallsThrough(t *testing.T) {
	// Only 1 of 4 labels found; threshold = (4+1)/2 = 2 → fall through.
	labels := []string{"COLUMNS", "FIRST_DATE", "LAST_DATE", "ROW_COUNT"}
	reasoning := `We need to start by looking at the data structure.

COLUMNS: date, open, high, low, close

Then I need to figure out the dates and row count separately by
running more analysis. Let me think about how to do that next.`

	_, ok := extractFinalAnswerWithLabels(reasoning, labels)
	if ok {
		t.Errorf("expected Strategy 4 to fall through with only 1/4 labels matched")
	}
}

func TestExtractFinalAnswerWithLabels_Strategy4_AllRequiredForSmallSchemas(t *testing.T) {
	// With ≤ 3 configured labels, ALL must be found.
	labels := []string{"NAME", "VALUE"}
	reasoningPartial := `NAME: test only the name was emitted`
	if _, ok := extractFinalAnswerWithLabels(reasoningPartial, labels); ok {
		t.Errorf("with ≤ 3 labels, all must be found; expected fall-through on 1/2")
	}

	reasoningComplete := `NAME: alpha
VALUE: beta`
	got, ok := extractFinalAnswerWithLabels(reasoningComplete, labels)
	if !ok {
		t.Fatal("expected complete 2/2 match to succeed")
	}
	if !strings.Contains(got, "NAME: alpha") || !strings.Contains(got, "VALUE: beta") {
		t.Errorf("expected both labels, got: %q", got)
	}
}

func TestExtractFinalAnswerWithLabels_Strategy4_IgnoresMidLineMentions(t *testing.T) {
	// "The COLUMNS for this CSV…" should NOT match — label must be at
	// the start of a line, followed immediately by a colon.
	labels := []string{"COLUMNS", "FIRST_DATE", "LAST_DATE"}
	reasoning := `The COLUMNS for this CSV include date and price information.
We need to think about FIRST_DATE and LAST_DATE separately as values.
None of these are properly enumerated yet so we should keep working.`

	_, ok := extractFinalAnswerWithLabels(reasoning, labels)
	if ok {
		t.Error("mid-line label mentions should NOT trigger Strategy 4")
	}
}

func TestExtractFinalAnswerWithLabels_Strategy4_RunsBeforeStrategy2(t *testing.T) {
	// When both Strategy 2 (trailing markdown) and Strategy 4 (labels)
	// would match, Strategy 4 wins — explicit user opt-in beats heuristic.
	labels := []string{"NAME", "VALUE"}
	reasoning := `Heuristic structural section follows.

## Final Report
- This is a markdown report
- With several bullets
- And looks committed

NAME: from-labels
VALUE: also-from-labels`

	got, ok := extractFinalAnswerWithLabels(reasoning, labels)
	if !ok {
		t.Fatal("expected extraction")
	}
	if !strings.Contains(got, "NAME: from-labels") {
		t.Errorf("expected Strategy 4 (label) to win, got: %q", got)
	}
	if strings.Contains(got, "## Final Report") {
		t.Errorf("Strategy 2 (markdown) should NOT have fired when labels are present, got: %q", got)
	}
}

func TestExtractFinalAnswerWithLabels_Strategy4_EmptyLabelsBehavesLikeNoLabels(t *testing.T) {
	// Sanity: nil and empty labels both disable Strategy 4 cleanly.
	reasoning := `Final answer: This is a long enough answer for Strategy 1 to commit to it.`
	for _, labels := range [][]string{nil, {}} {
		got, ok := extractFinalAnswerWithLabels(reasoning, labels)
		if !ok {
			t.Fatalf("labels=%v: expected Strategy 1 to still work", labels)
		}
		if !strings.Contains(got, "This is a long enough answer") {
			t.Errorf("labels=%v: expected Strategy 1 win, got: %q", labels, got)
		}
	}
}

func TestExtractFinalAnswerWithLabels_BackwardCompatibleWithExtractFinalAnswer(t *testing.T) {
	// extractFinalAnswer should be a thin wrapper for nil-labels.
	r := "Final answer: A long answer that satisfies the strict marker strategy length requirement."
	a, aOK := extractFinalAnswer(r)
	b, bOK := extractFinalAnswerWithLabels(r, nil)
	if aOK != bOK || a != b {
		t.Errorf("wrapper diverged: (%q,%v) vs (%q,%v)", a, aOK, b, bOK)
	}
}

func TestExtractByLabelPrefixes_EmptyLabels(t *testing.T) {
	if _, ok := extractByLabelPrefixes("anything", nil); ok {
		t.Error("nil labels should never match")
	}
	if _, ok := extractByLabelPrefixes("anything", []string{}); ok {
		t.Error("empty labels should never match")
	}
}

func TestExtractByLabelPrefixes_RegexSpecialCharactersInLabel(t *testing.T) {
	// Labels containing regex metacharacters must be escaped properly.
	// Real labels are ALL_CAPS_IDENTIFIER so this is purely defensive,
	// but verify regex composition doesn't blow up.
	labels := []string{"A.B", "C*D"}
	reasoning := `A.B: first value
C*D: second value`
	got, ok := extractByLabelPrefixes(reasoning, labels)
	if !ok {
		t.Fatal("expected escaped labels to match literally")
	}
	if !strings.Contains(got, "A.B: first value") {
		t.Errorf("expected literal A.B match, got: %q", got)
	}
}

func TestScanPromptLabels(t *testing.T) {
	prompt := `You are a data analyst. Output the following fields:

COLUMNS: list of column names
FIRST_DATE: ISO date of first row
LAST_DATE: ISO date of last row
ROW_COUNT: integer row count

The user's question follows below.`

	got := scanPromptLabels(prompt)
	want := []string{"COLUMNS", "FIRST_DATE", "LAST_DATE", "ROW_COUNT"}
	if len(got) != len(want) {
		t.Fatalf("got %d labels, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestScanPromptLabels_DedupesAndIgnoresShortAllCaps(t *testing.T) {
	prompt := `Reply with:
AI: brief description
NAME: identifier
NAME: also identifier (duplicate; ignored)
This sentence has NAME: in the middle, ignore that one.`

	got := scanPromptLabels(prompt)
	// "AI" is 2 chars so below the min-length-3 floor; expect only "NAME".
	if len(got) != 1 || got[0] != "NAME" {
		t.Errorf("got %v, want [NAME]", got)
	}
}

func TestIsStructuredListItem(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"- bullet", true},
		{"* asterisk", true},
		{"+ plus", true},
		{"1. numbered", true},
		{"42. larger number", true},
		{"  - indented bullet", true},
		{"\t- tabbed bullet", true},
		{"1.no space", false},
		{"-no space after dash", false},
		{"plain text", false},
		{"", false},
		{"  ", false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := isStructuredListItem(c.in)
			if got != c.want {
				t.Errorf("isStructuredListItem(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
