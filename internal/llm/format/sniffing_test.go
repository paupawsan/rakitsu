package format

import (
	"strings"
	"testing"
)

// TestSniffing_CommitsToReasoningOnFirstReasoningDelta verifies that a single
// reasoning_content delta is enough to commit to ReasoningContentField — we
// should never need to buffer more than one delta to catch a reasoning model.
func TestSniffing_CommitsToReasoningOnFirstReasoningDelta(t *testing.T) {
	a := Sniffing{}
	state := NewState()

	surface := a.ApplyDelta(state, RawDelta{ReasoningContent: "We"})
	// Buffered deltas replayed through the committed adapter; reasoning
	// tokens don't surface to the tracer (by design — tracer stays clean).
	if surface != "" {
		t.Errorf("reasoning-only delta should surface nothing, got %q", surface)
	}

	if name := a.CommittedName(state); name != "reasoning_content_field" {
		t.Errorf("after reasoning delta, committed adapter = %q, want reasoning_content_field", name)
	}
}

// TestSniffing_CommitsToStandardOnFirstContentDelta: any content delta
// commits to StandardOpenAI immediately (model is behaving normally).
func TestSniffing_CommitsToStandardOnFirstContentDelta(t *testing.T) {
	a := Sniffing{}
	state := NewState()

	surface := a.ApplyDelta(state, RawDelta{Content: "Hello "})
	if surface != "Hello " {
		t.Errorf("content delta should surface immediately, got %q", surface)
	}

	if name := a.CommittedName(state); name != "standard_openai" {
		t.Errorf("committed adapter = %q, want standard_openai", name)
	}

	// Subsequent deltas should pass through.
	surface = a.ApplyDelta(state, RawDelta{Content: "world"})
	if surface != "world" {
		t.Errorf("post-commit delta should pass through, got %q", surface)
	}

	r := a.Finalize(state)
	if r.Content != "Hello world" {
		t.Errorf("final content = %q, want %q", r.Content, "Hello world")
	}
}

// TestSniffing_ReasoningThenContentExtractsFinalAnswer: a reasoning model
// that emits reasoning_content first then eventually emits content chunks
// is still handled by ReasoningContentField, which preserves reasoning
// AND propagates the late content correctly.
func TestSniffing_ReasoningThenContentKeepsReasoningAdapter(t *testing.T) {
	a := Sniffing{}
	state := NewState()

	a.ApplyDelta(state, RawDelta{ReasoningContent: "Let me think. "})
	// Committed to ReasoningContentField. Now a late content chunk arrives.
	a.ApplyDelta(state, RawDelta{Content: "The answer is 42."})
	a.ApplyDelta(state, RawDelta{FinishReason: "stop"})

	if name := a.CommittedName(state); name != "reasoning_content_field" {
		t.Errorf("committed adapter = %q, want reasoning_content_field", name)
	}

	r := a.Finalize(state)
	if r.Content != "The answer is 42." {
		t.Errorf("content = %q, want exact late content", r.Content)
	}
	if !strings.Contains(r.Reasoning, "Let me think") {
		t.Errorf("reasoning should be preserved, got %q", r.Reasoning)
	}
}

// TestSniffing_BufferOverflowCommitsToStandard: if neither content nor
// reasoning_content appears within maxBufferDeltas, commit to Standard.
// This prevents infinite buffering on pathological streams.
func TestSniffing_BufferOverflowCommitsToStandard(t *testing.T) {
	a := Sniffing{}
	state := NewState()

	// Send maxBufferDeltas empty deltas (no content, no reasoning, no tools).
	for i := 0; i < maxBufferDeltas; i++ {
		a.ApplyDelta(state, RawDelta{FinishReason: ""})
	}

	if name := a.CommittedName(state); name != "standard_openai" {
		t.Errorf("after buffer overflow, committed = %q, want standard_openai", name)
	}
}

// TestSniffing_ToolCallFirstCommitsToStandard: a tool call delta with no
// content/reasoning commits to StandardOpenAI (tool calls are shape-
// identical in both adapters; Standard is the simpler default).
func TestSniffing_ToolCallFirstCommitsToStandard(t *testing.T) {
	a := Sniffing{}
	state := NewState()

	a.ApplyDelta(state, RawDelta{
		ToolCalls: []RawDeltaToolCall{
			{Index: 0, ID: "c1", Name: "search", Arguments: `{"q":"test"}`},
		},
	})

	if name := a.CommittedName(state); name != "standard_openai" {
		t.Errorf("committed = %q, want standard_openai", name)
	}

	r := a.Finalize(state)
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Name != "search" {
		t.Errorf("tool call should be preserved, got %+v", r.ToolCalls)
	}
}

// TestSniffing_BufferedContentReplayedThroughChosenAdapter: when sniffing
// buffers a content delta and then sees reasoning_content later — wait,
// actually this can't happen: the FIRST delta with content commits to
// Standard, so there's never a pre-commit delta with content that gets
// replayed under a different adapter. But we DO replay buffered reasoning
// deltas through ReasoningContentField — verify that works.
func TestSniffing_BufferedReasoningReplayedIntoState(t *testing.T) {
	a := Sniffing{}
	state := NewState()

	// First delta has both reasoning_content AND a FinishReason — the
	// reasoning signal wins. Replay should accumulate the reasoning.
	a.ApplyDelta(state, RawDelta{ReasoningContent: "thinking step 1. "})
	a.ApplyDelta(state, RawDelta{ReasoningContent: "Thus: the final answer is 42 across all dimensions."})

	r := a.Finalize(state)
	if !strings.Contains(r.Reasoning, "thinking step 1") {
		t.Errorf("buffered first reasoning delta should be in state.Reasoning, got %q", r.Reasoning)
	}
	if !strings.Contains(r.Content, "42 across all dimensions") {
		t.Errorf("final-answer extraction should work after sniff commit, got %q", r.Content)
	}
}

// TestSniffing_NonStreamingFullMessage: ApplyFull should pick the right
// adapter based on which fields are populated, no buffering needed.
func TestSniffing_NonStreamingContent(t *testing.T) {
	a := Sniffing{}
	state := NewState()
	a.ApplyFull(state, RawMessage{Content: "Hello", FinishReason: "stop"})
	if name := a.CommittedName(state); name != "standard_openai" {
		t.Errorf("full-content message should commit to standard_openai, got %s", name)
	}
	r := a.Finalize(state)
	if r.Content != "Hello" {
		t.Errorf("content = %q, want Hello", r.Content)
	}
}

func TestSniffing_NonStreamingReasoning(t *testing.T) {
	a := Sniffing{}
	state := NewState()
	a.ApplyFull(state, RawMessage{
		ReasoningContent: "Let me think. Final answer: The value is 42 units exactly.",
		FinishReason:     "length",
	})
	if name := a.CommittedName(state); name != "reasoning_content_field" {
		t.Errorf("full-reasoning message should commit to reasoning_content_field, got %s", name)
	}
	r := a.Finalize(state)
	if !strings.Contains(r.Content, "42 units exactly") {
		t.Errorf("reasoning extraction should succeed, got %q", r.Content)
	}
}

// TestSniffing_EmptyStreamFallsBackToStandard: if the stream ends without
// ever sending a discriminating delta, Finalize should still produce a
// valid (empty) result without panicking.
func TestSniffing_EmptyStreamSafeFallback(t *testing.T) {
	a := Sniffing{}
	state := NewState()
	r := a.Finalize(state)
	if r.Content != "" {
		t.Errorf("empty stream content = %q, want empty", r.Content)
	}
	if name := a.CommittedName(state); name != "standard_openai" {
		t.Errorf("empty stream should commit to standard_openai default, got %s", name)
	}
}
