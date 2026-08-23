package telemetry

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// ANSI color codes
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorCyan    = "\033[36m"
	colorMagenta = "\033[35m"
	colorGray    = "\033[90m"
	colorBold    = "\033[1m"
)

// ConsoleTracer subscribes to the event bus and prints formatted trace output to stderr.
type ConsoleTracer struct {
	eventCh    <-chan AgentEvent
	stopCh     chan struct{}
	stopOnce   sync.Once
	done       chan struct{}
	startTime  time.Time
	depth      int
	agentDepth map[string]int
	useColor   bool
	streaming  bool      // true while receiving TOKEN_CHUNK or REASONING_CHUNK events
	streamBuf  string    // accumulated streaming text for in-place preview
	streamKind string    // "answer" or "reasoning" — drives icon + cleared on kind change
	termWidth  int       // terminal width for truncation
	out        io.Writer // sink for formatted trace output (defaults to os.Stderr)
	mu         sync.Mutex
}

// NewConsoleTracer creates a new console tracer that writes to os.Stderr.
func NewConsoleTracer() *ConsoleTracer {
	return NewConsoleTracerWithWriter(os.Stderr)
}

// NewConsoleTracerWithWriter creates a new console tracer that writes to the
// provided writer. Used by tests to capture trace output without redirecting
// process-wide stderr. Color detection uses os.Stderr regardless — color
// codes are only emitted if the real stderr is a terminal, so tests reading
// from a buffer get plain text automatically.
func NewConsoleTracerWithWriter(w io.Writer) *ConsoleTracer {
	width := 80
	if tw, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && tw > 0 {
		width = tw
	}
	return &ConsoleTracer{
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
		agentDepth: make(map[string]int),
		useColor:   term.IsTerminal(int(os.Stderr.Fd())),
		termWidth:  width,
		out:        w,
	}
}

// Start subscribes to the event bus and begins printing trace output.
func (t *ConsoleTracer) Start(eb *EventBus) {
	t.eventCh = eb.Subscribe()
	t.startTime = time.Now()
	go t.run()
}

// Stop signals the tracer to stop and waits for it to finish. Idempotent —
// a second call is a no-op instead of panicking on a double close.
func (t *ConsoleTracer) Stop() {
	t.stopOnce.Do(func() {
		close(t.stopCh)
		<-t.done
	})
}

func (t *ConsoleTracer) run() {
	defer close(t.done)
	for {
		select {
		case <-t.stopCh:
			return
		case event, ok := <-t.eventCh:
			if !ok {
				return
			}
			t.handleEvent(event)
		}
	}
}

func (t *ConsoleTracer) handleEvent(event AgentEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch event.EventType {
	case EventAgentStart:
		t.handleAgentStart(event)
	case EventAgentEnd:
		t.handleAgentEnd(event)
	case EventThoughtStart:
		t.handleThoughtStart(event)
	case EventThoughtEnd:
		t.handleThoughtEnd(event)
	case EventToolCallStart:
		t.handleToolCallStart(event)
	case EventToolCallEnd:
		t.handleToolCallEnd(event)
	case EventAgentHandoff:
		t.handleAgentHandoff(event)
	case EventWorkerRedelegationBlocked:
		t.handleWorkerRedelegationBlocked(event)
	case EventAgentMessage:
		t.handleAgentMessage(event)
	case EventReflectionStart:
		t.handleReflectionStart(event)
	case EventReflectionEnd:
		t.handleReflectionEnd(event)
	case EventGroundCheckStart:
		t.handleGroundCheckStart(event)
	case EventGroundCheckEnd:
		t.handleGroundCheckEnd(event)
	case EventTokenChunk:
		t.handleTokenChunk(event)
	case EventReasoningChunk:
		t.handleReasoningChunk(event)
	case EventTokenUsage:
		t.handleTokenUsage(event)
	case EventDebugPaused:
		t.handleDebugPaused(event)
	case EventDebugResumed:
		t.handleDebugResumed(event)
	case EventError:
		t.handleError(event)
	case EventPipelineStart:
		t.handlePipelineStart(event)
	case EventPipelineEnd:
		t.handlePipelineEnd(event)
	case EventPipelineStepStart:
		t.handlePipelineStepStart(event)
	case EventPipelineStepEnd:
		t.handlePipelineStepEnd(event)
	case EventFormatSelected:
		t.handleFormatSelected(event)
	}
}

// parentDepth returns the depth for an event carrying an explicit spawn
// parent (parent's recorded depth + 1), or -1 when order-based depth applies.
func (t *ConsoleTracer) parentDepth(event AgentEvent) int {
	if event.ParentID == "" {
		return -1
	}
	if pd, ok := t.agentDepth[event.ParentID]; ok {
		return pd + 1
	}
	return -1
}

func (t *ConsoleTracer) handleAgentStart(event AgentEvent) {
	var p AgentStartPayload
	json.Unmarshal(event.Payload, &p)

	if pd := t.parentDepth(event); pd >= 0 {
		t.depth = pd
	}
	t.agentDepth[event.AgentName] = t.depth
	t.print(event, t.colorize(colorCyan, "▶"), fmt.Sprintf(
		"Agent %s started (%s, %s)",
		t.colorize(colorBold+colorCyan, fmt.Sprintf("%q", event.AgentName)),
		p.Model, p.Role,
	))
	t.depth++
}

func (t *ConsoleTracer) handleAgentEnd(event AgentEvent) {
	var p AgentEndPayload
	json.Unmarshal(event.Payload, &p)

	if d, ok := t.agentDepth[event.AgentName]; ok {
		t.depth = d
		delete(t.agentDepth, event.AgentName)
	} else if t.depth > 0 {
		t.depth--
	}

	status := t.colorize(colorGreen, p.Status)
	if p.Status != "success" {
		status = t.colorize(colorRed, p.Status)
	}

	extra := ""
	if p.TotalCost > 0 {
		extra += fmt.Sprintf(", $%.4f", p.TotalCost)
		if p.MaxCost > 0 {
			extra += fmt.Sprintf("/$%.2f", p.MaxCost)
		}
	}
	if p.MaxTokens > 0 {
		extra += fmt.Sprintf(", %s/%s budget", formatTokens(p.TotalTokens), formatTokens(p.MaxTokens))
	}

	t.print(event, t.colorize(colorCyan, "◀"), fmt.Sprintf(
		"Agent %s done (%s, %d iter, %s tokens%s)",
		t.colorize(colorBold+colorCyan, fmt.Sprintf("%q", event.AgentName)),
		status, p.Iterations, formatTokens(p.TotalTokens), extra,
	))
}

func (t *ConsoleTracer) handleThoughtStart(event AgentEvent) {
	var p ThoughtStartPayload
	json.Unmarshal(event.Payload, &p)

	t.print(event, t.colorize(colorGray, "◆"), fmt.Sprintf(
		"Thinking... (iteration %d, %d tools)",
		p.Iteration, len(p.AvailableTools),
	))
}

func (t *ConsoleTracer) handleThoughtEnd(event AgentEvent) {
	var p ThoughtEndPayload
	json.Unmarshal(event.Payload, &p)

	// Clear streaming preview line if active
	t.clearStreamLine()

	dur := formatDuration(event.Duration)
	action := "final answer"
	if len(p.IntendedToolCalls) > 0 {
		action = fmt.Sprintf("%d tool call(s)", len(p.IntendedToolCalls))
	}

	t.print(event, t.colorize(colorGray, "◆"), fmt.Sprintf(
		"Thought complete (%s) → %s", dur, action,
	))
}

func (t *ConsoleTracer) handleTokenChunk(event AgentEvent) {
	var p TokenChunkPayload
	json.Unmarshal(event.Payload, &p)

	if p.Text == "" {
		return
	}

	// If we were previously rendering a different stream kind (reasoning),
	// drop the line and reset before appending answer text.
	if t.streaming && t.streamKind != "answer" {
		t.clearStreamLine()
	}

	t.streaming = true
	t.streamKind = "answer"
	t.streamBuf += p.Text

	preview := strings.ReplaceAll(t.streamBuf, "\n", " ")
	elapsed := event.Timestamp.Sub(t.startTime).Seconds()
	timestamp := t.colorize(colorGray, fmt.Sprintf("[+%.1fs]", elapsed))
	indent := strings.Repeat("  ", t.depth)
	icon := t.colorize(colorGray, "·")
	prefix := fmt.Sprintf("%s %s%s ", timestamp, indent, icon)

	maxPreview := t.termWidth - len(stripAnsi(prefix)) - 1
	if maxPreview < 10 {
		maxPreview = 10
	}
	if len(preview) > maxPreview {
		preview = preview[len(preview)-maxPreview:]
	}

	fmt.Fprintf(t.out, "\r%s%s", prefix, t.colorize(colorGray, preview))
}

// handleReasoningChunk renders the reasoning_content stream in its own
// in-place lane. Distinct icon (… for "thinking") + same gray dim so it
// doesn't dominate the trace but is visible. Without this handler, a long
// reasoning phase looks identical to a stalled connection.
func (t *ConsoleTracer) handleReasoningChunk(event AgentEvent) {
	var p ReasoningChunkPayload
	json.Unmarshal(event.Payload, &p)

	if p.Text == "" {
		return
	}

	// Reset the preview line if we were previously rendering answer text.
	if t.streaming && t.streamKind != "reasoning" {
		t.clearStreamLine()
	}

	t.streaming = true
	t.streamKind = "reasoning"
	t.streamBuf += p.Text

	preview := strings.ReplaceAll(t.streamBuf, "\n", " ")
	elapsed := event.Timestamp.Sub(t.startTime).Seconds()
	timestamp := t.colorize(colorGray, fmt.Sprintf("[+%.1fs]", elapsed))
	indent := strings.Repeat("  ", t.depth)
	// "…" = thinking, distinct from "·" used by answer-stream chunks.
	icon := t.colorize(colorGray, "…")
	prefix := fmt.Sprintf("%s %s%s ", timestamp, indent, icon)

	maxPreview := t.termWidth - len(stripAnsi(prefix)) - 1
	if maxPreview < 10 {
		maxPreview = 10
	}
	if len(preview) > maxPreview {
		preview = preview[len(preview)-maxPreview:]
	}

	fmt.Fprintf(t.out, "\r%s%s", prefix, t.colorize(colorGray, preview))
}

// clearStreamLine clears the in-place streaming preview and resets state.
func (t *ConsoleTracer) clearStreamLine() {
	if !t.streaming {
		return
	}
	// Overwrite the line with spaces and return cursor
	fmt.Fprintf(t.out, "\r%s\r", strings.Repeat(" ", t.termWidth))
	t.streaming = false
	t.streamBuf = ""
}

// stripAnsi removes ANSI escape codes for length calculation.
func stripAnsi(s string) string {
	// Simple removal of \033[...m sequences
	result := strings.Builder{}
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until 'm'
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

func (t *ConsoleTracer) handleToolCallStart(event AgentEvent) {
	var p ToolCallStartPayload
	json.Unmarshal(event.Payload, &p)

	args := formatArgs(p.Arguments)
	t.print(event, t.colorize(colorYellow, "⚡"), fmt.Sprintf(
		"%s(%s)",
		t.colorize(colorBold+colorYellow, p.ToolName), args,
	))
}

func (t *ConsoleTracer) handleToolCallEnd(event AgentEvent) {
	var p ToolCallEndPayload
	json.Unmarshal(event.Payload, &p)

	dur := formatDuration(event.Duration)
	if p.Error != "" {
		t.print(event, t.colorize(colorRed, "✗"), fmt.Sprintf(
			"%s failed (%s): %s",
			t.colorize(colorYellow, p.ToolName), dur, truncate(p.Error, 100),
		))
		// B14a fix (2026-04-05): surface tool stderr/stdout to the operator.
		// Before this, the trace only showed the wrapped err.Error() like
		// "command failed: exit status 1", hiding the actual reason for the
		// failure (which CombinedOutput captures into p.Output). Dogfood run
		// on 2026-04-05 spent 11 retry iterations because the operator
		// couldn't see WHY git was failing. The fix is to also render the
		// first few lines of p.Output prefixed with "│ " per line.
		if p.Output != "" {
			t.printToolOutputBlock(event, p.Output, toolErrorOutputMaxBytes, toolErrorOutputMaxLines)
		}
	} else {
		size := formatBytes(len(p.Output))
		t.print(event, t.colorize(colorGreen, "✓"), fmt.Sprintf(
			"%s (%s, %s)",
			t.colorize(colorYellow, p.ToolName), dur, size,
		))
	}
}

// Caps for B14a stderr block. Kept small so the trace stays scannable — the
// full output is still in the session log and telemetry event for anyone
// who needs the whole thing.
const (
	toolErrorOutputMaxBytes = 500 // ~5-8 lines of typical stderr
	toolErrorOutputMaxLines = 6
)

// printToolOutputBlock renders the first maxLines (up to maxBytes) of a
// failed tool's output as a continuation block after the failure line,
// with each line prefixed by "│ " so it visually belongs to the trace
// entry above it. Long lines are truncated to termWidth.
func (t *ConsoleTracer) printToolOutputBlock(event AgentEvent, output string, maxBytes, maxLines int) {
	// Strip trailing whitespace so we don't end on a blank line
	output = strings.TrimRight(output, " \t\n\r")
	if output == "" {
		return
	}

	// Apply byte cap first (in case stderr is a multi-megabyte dump)
	truncated := false
	if len(output) > maxBytes {
		output = output[:maxBytes]
		truncated = true
	}

	lines := strings.Split(output, "\n")
	// Apply line cap
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}

	elapsed := event.Timestamp.Sub(t.startTime).Seconds()
	timestamp := t.colorize(colorGray, fmt.Sprintf("[+%.1fs]", elapsed))
	indent := strings.Repeat("  ", t.depth)
	barColor := t.colorize(colorRed, "│")

	// Max width for content after the prefix
	prefixVisual := fmt.Sprintf("[+%.1fs] %s│ ", elapsed, indent)
	maxLineWidth := t.termWidth - len(prefixVisual) - 1
	if maxLineWidth < 20 {
		maxLineWidth = 20
	}

	for _, line := range lines {
		if len(line) > maxLineWidth {
			line = line[:maxLineWidth-1] + "…"
		}
		fmt.Fprintf(t.out, "%s %s%s %s\n", timestamp, indent, barColor, line)
	}

	if truncated {
		fmt.Fprintf(t.out, "%s %s%s %s\n", timestamp, indent, barColor,
			t.colorize(colorGray, "… (output truncated, see session log for full stderr)"))
	}
}

func (t *ConsoleTracer) handleAgentHandoff(event AgentEvent) {
	var p AgentHandoffPayload
	json.Unmarshal(event.Payload, &p)

	t.print(event, t.colorize(colorCyan, "⤷"), fmt.Sprintf(
		"Handoff → %s: %s",
		t.colorize(colorBold+colorCyan, fmt.Sprintf("%q", p.ToAgent)),
		truncate(p.Task, 80),
	))
}

func (t *ConsoleTracer) handleAgentMessage(event AgentEvent) {
	var p AgentMessagePayload
	json.Unmarshal(event.Payload, &p)

	t.print(event, t.colorize(colorCyan, "⤶"), fmt.Sprintf(
		"Result from %s (%s)",
		t.colorize(colorBold+colorCyan, fmt.Sprintf("%q", p.FromAgent)),
		formatBytes(len(p.Message)),
	))
}

func (t *ConsoleTracer) handleWorkerRedelegationBlocked(event AgentEvent) {
	var p WorkerRedelegationBlockedPayload
	json.Unmarshal(event.Payload, &p)

	t.print(event, t.colorize(colorYellow, "⊘"), fmt.Sprintf(
		"Delegation blocked → %s (%s, post-salvage count %d)",
		t.colorize(colorBold+colorYellow, fmt.Sprintf("%q", p.BlockedWorker)),
		p.Reason,
		p.PostSalvageCount,
	))
}

func (t *ConsoleTracer) handleReflectionStart(event AgentEvent) {
	var p ReflectionStartPayload
	json.Unmarshal(event.Payload, &p)

	t.print(event, t.colorize(colorMagenta, "↻"), fmt.Sprintf(
		"Reflecting (%s)...", p.Mode,
	))
}

func (t *ConsoleTracer) handleReflectionEnd(event AgentEvent) {
	dur := formatDuration(event.Duration)
	t.print(event, t.colorize(colorMagenta, "↻"), fmt.Sprintf(
		"Reflection done (%s)", dur,
	))
}

func (t *ConsoleTracer) handleGroundCheckStart(event AgentEvent) {
	t.print(event, t.colorize(colorMagenta, "✔"), "Ground-check...")
}

func (t *ConsoleTracer) handleGroundCheckEnd(event AgentEvent) {
	var p GroundCheckEndPayload
	json.Unmarshal(event.Payload, &p)

	valid := t.colorize(colorGreen, "valid")
	if !p.IsValid {
		valid = t.colorize(colorRed, "invalid")
	}

	t.print(event, t.colorize(colorMagenta, "✔"), fmt.Sprintf(
		"Ground-check: %.2f confidence, %s → %s",
		p.Confidence, valid, p.Action,
	))
}

func (t *ConsoleTracer) handleTokenUsage(event AgentEvent) {
	if event.TokenUsage == nil {
		return
	}
	u := event.TokenUsage
	t.print(event, t.colorize(colorGray, "⊘"), fmt.Sprintf(
		"Tokens: %s in / %s out / %s total",
		formatTokens(u.InputTokens), formatTokens(u.OutputTokens), formatTokens(u.TotalTokens),
	))
}

func (t *ConsoleTracer) handleDebugPaused(event AgentEvent) {
	var p DebugPausedPayload
	json.Unmarshal(event.Payload, &p)

	detail := fmt.Sprintf(
		"PAUSED at %s (agent: %s, iter: %d, reason: %s)",
		t.colorize(colorBold+colorRed, p.Checkpoint), p.AgentName, p.Iteration, p.Reason,
	)

	// Context fields
	if p.HistoryLength > 0 || p.TotalTokensIn > 0 {
		detail += fmt.Sprintf("\n         History: %d msgs, Tokens: %s in / %s out",
			p.HistoryLength, formatTokens(p.TotalTokensIn), formatTokens(p.TotalTokensOut))
	}
	if p.BudgetRatio > 0 {
		detail += fmt.Sprintf(", Budget: %.0f%%", p.BudgetRatio*100)
	}
	if len(p.PendingTools) > 0 {
		names := make([]string, len(p.PendingTools))
		for i, pt := range p.PendingTools {
			names[i] = pt.Name
		}
		detail += fmt.Sprintf("\n         Pending: %s", strings.Join(names, ", "))
	}
	if p.LastThought != "" {
		thought := p.LastThought
		if len(thought) > 200 {
			thought = thought[:200] + "..."
		}
		detail += fmt.Sprintf("\n         Thought: %s", thought)
	}

	t.print(event, t.colorize(colorRed, "⏸"), detail)
}

func (t *ConsoleTracer) handleDebugResumed(event AgentEvent) {
	var p DebugResumedPayload
	json.Unmarshal(event.Payload, &p)
	t.print(event, t.colorize(colorGreen, "▶"), fmt.Sprintf(
		"RESUMED (action: %s)", p.Action,
	))
}

func (t *ConsoleTracer) handleError(event AgentEvent) {
	var p ErrorPayload
	json.Unmarshal(event.Payload, &p)

	t.print(event, t.colorize(colorRed, "✗"), fmt.Sprintf(
		"Error [%s]: %s", p.ErrorType, truncate(p.Message, 120),
	))
}

func (t *ConsoleTracer) handlePipelineStart(event AgentEvent) {
	var p PipelineStartPayload
	json.Unmarshal(event.Payload, &p)

	t.print(event, t.colorize(colorCyan, "▶"), fmt.Sprintf(
		"Pipeline started (%d steps): %s",
		p.StepCount, truncate(p.Query, 80),
	))
	t.depth++
}

func (t *ConsoleTracer) handlePipelineEnd(event AgentEvent) {
	if t.depth > 0 {
		t.depth--
	}
	var p PipelineEndPayload
	json.Unmarshal(event.Payload, &p)

	status := t.colorize(colorGreen, p.Status)
	if p.Status != "success" {
		status = t.colorize(colorRed, p.Status)
	}
	t.print(event, t.colorize(colorCyan, "◀"), fmt.Sprintf(
		"Pipeline done (%s)", status,
	))
}

func (t *ConsoleTracer) handlePipelineStepStart(event AgentEvent) {
	var p PipelineStepStartPayload
	json.Unmarshal(event.Payload, &p)

	agents := ""
	if len(p.Agents) > 0 {
		agents = fmt.Sprintf(", agents: %s", strings.Join(p.Agents, ", "))
	}
	t.print(event, t.colorize(colorYellow, "▸"), fmt.Sprintf(
		"Step %s [%s%s]",
		t.colorize(colorBold+colorYellow, fmt.Sprintf("%q", p.StepName)),
		p.StepType, agents,
	))
	t.depth++
}

func (t *ConsoleTracer) handlePipelineStepEnd(event AgentEvent) {
	if t.depth > 0 {
		t.depth--
	}
	var p PipelineStepEndPayload
	json.Unmarshal(event.Payload, &p)

	dur := formatDuration(event.Duration)
	status := t.colorize(colorGreen, p.Status)
	if p.Status != "success" {
		status = t.colorize(colorRed, p.Status)
	}
	t.print(event, t.colorize(colorYellow, "▪"), fmt.Sprintf(
		"Step %s done (%s, %s)",
		t.colorize(colorBold+colorYellow, fmt.Sprintf("%q", p.StepName)),
		status, dur,
	))
}

// handleFormatSelected renders the response-format adapter decision for a
// given agent run. Fires once per run. Highlights when Sniffing committed
// because that's the case where the operator most wants to see which
// concrete adapter handled their request.
func (t *ConsoleTracer) handleFormatSelected(event AgentEvent) {
	var p FormatSelectedPayload
	json.Unmarshal(event.Payload, &p)

	reasonLabel := p.Reason
	switch p.Reason {
	case "pattern_match":
		reasonLabel = "matched model pattern"
	case "override":
		reasonLabel = "explicit config override"
	case "sniff_commit":
		reasonLabel = t.colorize(colorYellow, "auto-sniffed from first deltas")
	case "fallback":
		reasonLabel = "fallback (sniffing pending)"
	}
	t.print(event, t.colorize(colorCyan, "⚙"), fmt.Sprintf(
		"Format: %s (%s)",
		t.colorize(colorBold+colorCyan, p.Adapter),
		reasonLabel,
	))
}

// print writes a formatted trace line to stderr.
func (t *ConsoleTracer) print(event AgentEvent, icon, message string) {
	// Clear any active streaming preview before printing a structured line.
	// Without this, the \r-based token preview collides with the next event
	// line (e.g. tool_call_start), producing garbled output like
	// `list_files([+3.0s]   ·  list_files({`.
	t.clearStreamLine()
	elapsed := event.Timestamp.Sub(t.startTime).Seconds()
	timestamp := t.colorize(colorGray, fmt.Sprintf("[+%.1fs]", elapsed))
	indent := strings.Repeat("  ", t.depth)
	fmt.Fprintf(t.out, "%s %s%s %s\n", timestamp, indent, icon, message)
}

// colorize wraps text with ANSI color codes if color is enabled.
func (t *ConsoleTracer) colorize(color, text string) string {
	if !t.useColor {
		return text
	}
	return color + text + colorReset
}

// formatDuration formats milliseconds into a human-readable duration.
func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

// formatBytes formats byte count into a human-readable size.
func formatBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	return fmt.Sprintf("%.1fKB", float64(n)/1024)
}

// formatTokens formats token count.
func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fK", float64(n)/1000)
}

// formatArgs formats tool arguments for display. Keys are sorted so the
// rendered order is deterministic across runs — Go map iteration order is
// randomized, which would otherwise make trace output vary call to call.
func formatArgs(args map[string]interface{}) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(args))
	for _, k := range keys {
		s := fmt.Sprintf("%v", args[k])
		parts = append(parts, fmt.Sprintf("%s: %s", k, truncate(s, 40)))
	}
	return truncate(strings.Join(parts, ", "), 100)
}

// truncate shortens s to maxLen runes, adding ellipsis if truncated.
// Rune-safe: s is arbitrary LLM-generated text and routinely contains
// multi-byte characters, so byte-offset slicing would corrupt it.
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "..."
}
