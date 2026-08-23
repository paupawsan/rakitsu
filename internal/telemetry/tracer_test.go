package telemetry

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// makeChunkEvent constructs an AgentEvent with the given payload, used by the
// reasoning/answer rendering tests below.
func makeChunkEvent(t *testing.T, eventType EventType, payload interface{}) AgentEvent {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return AgentEvent{
		ID:        "evt-test",
		Timestamp: time.Now(),
		EventType: eventType,
		AgentName: "tester",
		Payload:   raw,
	}
}

// newTestTracer returns a ConsoleTracer wired to an in-memory buffer with a
// fixed terminal width so column-arithmetic assertions are deterministic. Color
// is forced off because we detect from real os.Stderr; tests must not assume a
// terminal is attached.
func newTestTracer() (*ConsoleTracer, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	tracer := NewConsoleTracerWithWriter(buf)
	tracer.startTime = time.Now().Add(-time.Second)
	tracer.useColor = false
	tracer.termWidth = 80
	return tracer, buf
}

// TestConsoleTracer_ReasoningChunk_RendersThinkingIcon verifies the audit
// Claim 1-2 fix: a REASONING_CHUNK event must render with the `…` icon so a
// long reasoning phase is visibly distinct from a stalled connection.
func TestConsoleTracer_ReasoningChunk_RendersThinkingIcon(t *testing.T) {
	tracer, buf := newTestTracer()

	tracer.handleEvent(makeChunkEvent(t, EventReasoningChunk, ReasoningChunkPayload{
		Text:      "Let me think about this",
		AgentName: "tester",
		Iteration: 1,
	}))

	out := buf.String()
	if !strings.Contains(out, "…") {
		t.Errorf("reasoning chunk should render with `…` icon; got %q", out)
	}
	if !strings.Contains(out, "Let me think about this") {
		t.Errorf("reasoning chunk should render its text; got %q", out)
	}
	if !tracer.streaming || tracer.streamKind != "reasoning" {
		t.Errorf("tracer state should be streaming=true, kind=reasoning; got %v / %q",
			tracer.streaming, tracer.streamKind)
	}
}

// TestConsoleTracer_TokenChunk_RendersAnswerIcon is the answer-stream peer to
// the reasoning test above. The two icons MUST differ so an operator can
// distinguish "model is thinking" from "model is writing the answer."
func TestConsoleTracer_TokenChunk_RendersAnswerIcon(t *testing.T) {
	tracer, buf := newTestTracer()

	tracer.handleEvent(makeChunkEvent(t, EventTokenChunk, TokenChunkPayload{
		Text:      "Here is the answer",
		AgentName: "tester",
		Iteration: 1,
	}))

	out := buf.String()
	if !strings.Contains(out, "·") {
		t.Errorf("answer chunk should render with `·` icon; got %q", out)
	}
	if strings.Contains(out, "…") {
		t.Errorf("answer chunk MUST NOT render the reasoning `…` icon; got %q", out)
	}
	if tracer.streamKind != "answer" {
		t.Errorf("streamKind = %q, want answer", tracer.streamKind)
	}
}

// TestConsoleTracer_KindSwitch_ClearsLine verifies the kind-switch contract:
// when an in-place reasoning preview is followed by an answer chunk (or vice
// versa), the previous buffer must be dropped so the two streams don't bleed
// into one another. Without this, the `…` icon stays on screen while answer
// text appends after it, producing garbled `…Hello world` lines.
func TestConsoleTracer_KindSwitch_ClearsLine(t *testing.T) {
	tracer, buf := newTestTracer()

	tracer.handleEvent(makeChunkEvent(t, EventReasoningChunk, ReasoningChunkPayload{
		Text:      "reasoning text",
		AgentName: "tester",
	}))
	if tracer.streamBuf == "" {
		t.Fatal("expected streamBuf to accumulate reasoning text")
	}

	tracer.handleEvent(makeChunkEvent(t, EventTokenChunk, TokenChunkPayload{
		Text:      "answer text",
		AgentName: "tester",
	}))

	// Kind-switch must have cleared the buffer of the prior stream, then
	// started the new one with only the answer chunk.
	if tracer.streamKind != "answer" {
		t.Errorf("after switch, streamKind = %q, want answer", tracer.streamKind)
	}
	if tracer.streamBuf != "answer text" {
		t.Errorf("after switch, streamBuf = %q, want %q (reasoning must be dropped)",
			tracer.streamBuf, "answer text")
	}

	// Both icons must appear in the buffered output: `…` from the reasoning
	// render, then a clear (spaces + \r), then `·` from the answer render.
	out := buf.String()
	if !strings.Contains(out, "…") || !strings.Contains(out, "·") {
		t.Errorf("output should contain both `…` and `·`; got %q", out)
	}
}

// TestConsoleTracer_EmptyChunks_NoOp ensures empty-text chunks (which providers
// can emit during keepalives) don't accidentally flip the streaming state and
// confuse the next real chunk's kind-switch logic.
func TestConsoleTracer_EmptyChunks_NoOp(t *testing.T) {
	tracer, buf := newTestTracer()

	tracer.handleEvent(makeChunkEvent(t, EventReasoningChunk, ReasoningChunkPayload{Text: ""}))
	tracer.handleEvent(makeChunkEvent(t, EventTokenChunk, TokenChunkPayload{Text: ""}))

	if tracer.streaming {
		t.Errorf("streaming should remain false on empty chunks; got streamKind=%q", tracer.streamKind)
	}
	if buf.Len() != 0 {
		t.Errorf("empty chunks should produce no output; got %q", buf.String())
	}
}

// TestConsoleTracerStopIsIdempotent: Stop used to close(stopCh)
// unconditionally, so a second call panicked with "close of closed channel".
func TestConsoleTracerStopIsIdempotent(t *testing.T) {
	tracer, _ := newTestTracer()
	tracer.Start(NewEventBus(8))

	done := make(chan struct{})
	go func() {
		tracer.Stop()
		tracer.Stop() // must not panic
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop() did not return twice within the timeout")
	}
}

// TestFormatArgsIsDeterministic: formatArgs used to range over the args map
// directly, so the rendered key order (and thus trace/log output) varied
// randomly run to run for the same call.
func TestFormatArgsIsDeterministic(t *testing.T) {
	args := map[string]interface{}{"zebra": 1, "path": "foo", "offset": 0, "alpha": "x"}
	want := formatArgs(args)
	for i := 0; i < 20; i++ {
		if got := formatArgs(args); got != want {
			t.Fatalf("formatArgs output changed across calls:\n  first: %q\n  now:   %q", want, got)
		}
	}
}

// TestTruncateIsRuneSafe: truncate used to slice by byte offset, which can
// split a multi-byte character and produce invalid UTF-8 in trace/log
// output built from arbitrary LLM-generated text.
func TestTruncateIsRuneSafe(t *testing.T) {
	s := strings.Repeat("要", 50) // 3-byte rune
	got := truncate(s, 10)
	if !utf8.ValidString(got) {
		t.Errorf("truncate produced invalid UTF-8: %q", got)
	}
	if want := strings.Repeat("要", 10) + "..."; got != want {
		t.Errorf("truncate(%d runes, 10) = %q, want %q", len([]rune(s)), got, want)
	}
}
