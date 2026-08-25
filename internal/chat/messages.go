// Package chat provides a rich terminal UI for interactive agent sessions.
package chat

import tea "github.com/charmbracelet/bubbletea"

// --- Bubbletea messages ---

// The `Directed` field several messages below carry reports that the event
// came from a directed agent chat turn (Roster.Send) rather than the main run.
// It is set from the event's own Origin tag (see telemetry.AgentEvent.Origin),
// never inferred from the agent name: the main run can be delegating to an
// agent at the very moment the user is talking to that same agent in a side
// thread, and a name test then drops the main run's rows. False — the zero
// value — means the main run, so a main-run message needs no ceremony.

// TokenChunkMsg delivers a streaming text chunk from the agent.
type TokenChunkMsg struct {
	Text      string
	AgentName string
	Directed  bool
}

// TokenUsageMsg reports per-LLM-call token usage. The chat Model
// accumulates Total into its visible counter so the status bar reflects
// running session cost.
type TokenUsageMsg struct {
	Input  int
	Output int
	Total  int
}

// AgentDoneMsg signals agent run completion (success or error).
// Token matches the submitQuery generation that spawned this run; stale
// AgentDoneMsg (from interrupted runs) have non-matching tokens and are
// ignored by the Update handler.
type AgentDoneMsg struct {
	Token    int
	Response string
	Err      error
}

// AgentChatDoneMsg signals a directed agent chat turn's completion. Agent
// names the thread it belongs to; Token matches the submitToAgent generation
// that spawned it, so a reply from an interrupted run is ignored the same way
// AgentDoneMsg's is. Threads settle independently — a reply for a thread the
// user is not looking at lands on that thread's stored blocks.
type AgentChatDoneMsg struct {
	Agent string
	Token int
	Reply string
	Err   error
}

// ReasoningChunkMsg delivers a streaming reasoning_content delta from a
// reasoning model (Nemotron, DeepSeek-R1, QwQ, ...). Rendered in a separate
// dimmed lane from the visible answer stream.
type ReasoningChunkMsg struct {
	Text      string
	AgentName string
	Directed  bool
}

// ToolCallStartMsg signals the start of a tool execution.
type ToolCallStartMsg struct {
	ID        string
	Name      string
	Args      string // JSON-formatted arguments
	AgentName string // emitting agent — drives block nesting
	Directed  bool
}

// ToolCallEndMsg signals tool execution completion. AgentName is the emitting
// agent, carried for the same reason ToolCallStartMsg carries it: the chat
// surface has to know which conversation an event belongs to.
type ToolCallEndMsg struct {
	ID        string
	Name      string
	Output    string
	Error     string
	Duration  int64 // milliseconds
	AgentName string
	Directed  bool
}

// AgentStartMsg signals a spawned subagent (or, when Parent=="", any
// top-level/orchestrator agent) started. Parent is set only for
// runtime-spawned children.
type AgentStartMsg struct {
	Name     string
	Parent   string
	Model    string
	Provider string
	Directed bool
}

// AgentEndMsg signals an agent finished (success, error, timeout,
// interrupted, ...). Fields mirror AgentEndPayload / event Duration.
// Cost/MaxTokens/MaxCost/PricingKnown are populated only when the agent has
// a TokenGuard attached (zero values otherwise).
type AgentEndMsg struct {
	Name         string
	Status       string
	Tokens       int
	Duration     int64
	Cost         float64
	MaxTokens    int
	MaxCost      float64
	Iterations   int
	PricingKnown bool
	Directed     bool
}

// StatusMsg is a transient status update.
type StatusMsg struct {
	Text string
}

// UserInputRequestMsg signals the user_input tool is waiting for a response.
type UserInputRequestMsg struct {
	Question string
	Default  string
}

// InboundSessionMsg carries a message injected from another live session
// (send_message tool / `rakitsu sessions send`), delivered via the hub's
// "session_message" command poll. It doubles as the tea.Msg the inbound
// channel pump emits. Never interrupts: if the main conversation is busy or
// a side thread is on screen, it queues behind the current activity. From
// fields are sender-claimed and unverified.
type InboundSessionMsg struct {
	FromSessionID string
	FromName      string
	Text          string
	// WaitToken, when non-empty, means the sender is synchronously waiting
	// for this turn's outcome — the result must be reported back to the hub
	// (see Config.PostMessageResult) once the resulting AgentDoneMsg lands.
	// Empty means fire-and-forget, the default.
	WaitToken string
}

// tickMsg drives spinner animation.
type tickMsg struct{}

// waitForEvent returns a tea.Cmd that blocks on the event channel.
func waitForEvent(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}
