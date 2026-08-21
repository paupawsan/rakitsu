package format

import "strings"

// Sniffing is a wrapping adapter that auto-detects the correct underlying
// adapter by inspecting the first few streaming deltas. It is used as the
// default fallback when the model-name registry doesn't have a match for a
// given model — without sniffing, unknown reasoning models (any new model
// family we haven't registered yet) would silently produce broken output.
//
// Lifecycle:
//   1. State = buffering. Each ApplyDelta call appends to the buffer AND
//      returns no surface text (tracer stays quiet).
//   2. On each delta, Sniffing inspects the shape:
//        - reasoning_content non-empty → commit to ReasoningContentField
//        - content non-empty → commit to StandardOpenAI
//        - tool_calls present → commit to StandardOpenAI (both adapters
//          handle tool calls identically via StandardOpenAI's helpers)
//        - buffer grew past maxBufferDeltas → commit to StandardOpenAI
//          (conservative fallback — safer than assuming reasoning model)
//   3. On commit:
//        - Replay all buffered deltas through the chosen inner adapter in
//          order, capturing the surface text each one produces.
//        - Concatenate that surface text and return it to the caller. The
//          caller (provider stream loop) emits it as a single StreamChunk
//          so the operator sees the delayed-but-correct trace output.
//        - Record the committed adapter so the non-streaming path and the
//          telemetry layer can ask "which adapter was actually used?".
//   4. After commit, all subsequent ApplyDelta calls delegate directly to
//      the committed inner adapter — the buffer is unused.
//
// For non-streaming ApplyFull, Sniffing picks the inner adapter synchronously
// based on which field is populated on the full message. No buffering needed.
//
// The committed adapter is exposed via CommittedName() for telemetry. Before
// commit it returns "sniffing_pending".
type Sniffing struct{}

func (Sniffing) Name() string { return "sniffing" }

// maxBufferDeltas caps how many deltas we'll hold before forcing a commit
// to StandardOpenAI. In practice, reasoning_content or content lands on
// delta 0 or 1 for every model we've seen, so this is a safety net not a
// common path. A small number keeps tracer latency negligible.
const maxBufferDeltas = 5

// sniffingScratch is the per-request state specific to Sniffing. It lives
// inside FormatState.Scratch as an interface{} so the generic FormatState
// doesn't need to know about sniffing internals.
type sniffingScratch struct {
	committed ResponseFormat // nil until commit
	buffer    []RawDelta
}

func (Sniffing) ApplyDelta(state *FormatState, delta RawDelta) string {
	scratch := getSniffScratch(state)

	// Already committed — just delegate.
	if scratch.committed != nil {
		return scratch.committed.ApplyDelta(state, delta)
	}

	// Still sniffing — buffer this delta first, then decide.
	scratch.buffer = append(scratch.buffer, delta)

	chosen := decideFromDelta(delta)
	if chosen == nil && len(scratch.buffer) >= maxBufferDeltas {
		// Buffer overflow without a signal — conservative fallback.
		chosen = StandardOpenAI{}
	}
	if chosen == nil {
		// Still undecided; keep buffering silently.
		return ""
	}

	// Commit. Replay the buffered deltas through the chosen adapter and
	// concatenate the surface text so the caller can emit one catch-up chunk.
	scratch.committed = chosen
	var surface strings.Builder
	for _, d := range scratch.buffer {
		if s := chosen.ApplyDelta(state, d); s != "" {
			surface.WriteString(s)
		}
	}
	scratch.buffer = nil // release memory; no longer needed
	return surface.String()
}

func (Sniffing) ApplyFull(state *FormatState, msg RawMessage) {
	// Non-streaming: pick synchronously based on which fields are populated.
	scratch := getSniffScratch(state)
	var chosen ResponseFormat = StandardOpenAI{}
	if msg.Content == "" && msg.ReasoningContent != "" {
		chosen = ReasoningContentField{}
	} else if msg.ReasoningContent != "" {
		// Both present — reasoning model with content. ReasoningContentField
		// handles this correctly (content takes precedence, reasoning accumulates).
		chosen = ReasoningContentField{}
	}
	scratch.committed = chosen
	chosen.ApplyFull(state, msg)
}

func (Sniffing) Finalize(state *FormatState) Result {
	scratch := getSniffScratch(state)
	if scratch.committed == nil {
		// Stream ended before we saw any discriminating signal — usually
		// means an empty response or a pure-tool-call response that never
		// produced content or reasoning text. Safe fallback: StandardOpenAI.
		// Replay any buffered deltas just in case there were tool-call fragments.
		chosen := StandardOpenAI{}
		for _, d := range scratch.buffer {
			chosen.ApplyDelta(state, d)
		}
		scratch.committed = chosen
		scratch.buffer = nil
	}
	return scratch.committed.Finalize(state)
}

// CommittedName returns the name of the adapter Sniffing committed to, or
// "sniffing_pending" if no commit has happened yet. Used for telemetry so
// operators can see which concrete adapter handled a given request.
func (Sniffing) CommittedName(state *FormatState) string {
	scratch := getSniffScratch(state)
	if scratch.committed == nil {
		return "sniffing_pending"
	}
	return scratch.committed.Name()
}

// decideFromDelta returns the adapter to commit to based on a single delta,
// or nil if the delta doesn't provide enough signal yet.
func decideFromDelta(d RawDelta) ResponseFormat {
	if d.ReasoningContent != "" {
		return ReasoningContentField{}
	}
	if d.Content != "" {
		return StandardOpenAI{}
	}
	if len(d.ToolCalls) > 0 {
		// Tool calls land identically in both adapters; picking Standard is
		// the simpler default. If a later delta adds reasoning_content before
		// Finalize, we're already committed — but that's fine because tool
		// calls don't depend on reasoning extraction.
		return StandardOpenAI{}
	}
	return nil
}

// getSniffScratch retrieves or initializes the sniffing-specific scratch
// storage from FormatState. Panics if another adapter stored something
// incompatible in Scratch (shouldn't happen in practice — only one adapter
// per request).
func getSniffScratch(state *FormatState) *sniffingScratch {
	if state.Scratch == nil {
		s := &sniffingScratch{}
		state.Scratch = s
		return s
	}
	s, ok := state.Scratch.(*sniffingScratch)
	if !ok {
		// Another adapter owned the scratch slot — this is a programming
		// error (two adapters sharing one state), make it loud.
		panic("format.Sniffing: FormatState.Scratch is owned by a different adapter")
	}
	return s
}
