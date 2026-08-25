package chat

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
)

// TestStatusBar_Idle shows the default prompt when the chat is idle.
func TestStatusBar_Idle(t *testing.T) {
	m := Model{width: 80, spinner: spinner.New()}
	s := m.statusBar()
	if !strings.Contains(s, "Enter send") {
		t.Errorf("idle status should show the send hint, got %q", s)
	}
	if !strings.Contains(s, "Ctrl+Y copy") {
		t.Errorf("idle status should show the copy hint, got %q", s)
	}
	if strings.Contains(s, "generating") {
		t.Errorf("idle status should NOT show 'generating', got %q", s)
	}
}

// TestStatusBar_Notice shows a transient notice (e.g. after a copy) in place
// of the default idle hint.
func TestStatusBar_Notice(t *testing.T) {
	m := Model{width: 80, spinner: spinner.New(), notice: "copied reply to clipboard"}
	s := m.statusBar()
	if !strings.Contains(s, "copied reply to clipboard") {
		t.Errorf("status should show the notice, got %q", s)
	}
}

// TestStatusBar_Generating shows the spinner + generating while an LLM call
// is in flight.
func TestStatusBar_Generating(t *testing.T) {
	m := Model{width: 80, spinner: spinner.New(), generating: true}
	s := m.statusBar()
	if !strings.Contains(s, "generating") {
		t.Errorf("generating status should show 'generating', got %q", s)
	}
}

// TestStatusBar_WaitingForInput is the key fix: when the user_input tool is
// pending a response, the status must say so instead of the misleading
// 'generating...' label. The agent is paused on a channel recv,
// not burning tokens.
func TestStatusBar_WaitingForInput(t *testing.T) {
	m := Model{width: 80, spinner: spinner.New(), generating: true, waitingForInput: true}
	s := m.statusBar()
	if strings.Contains(s, "generating") {
		t.Errorf("waitingForInput status must NOT show 'generating', got %q", s)
	}
	if !strings.Contains(s, "waiting") {
		t.Errorf("waitingForInput status should indicate waiting, got %q", s)
	}
}

// TestTokenUsageMsg_AccumulatesTotalTokens verifies that per-call
// token usage events sum into the visible counter. Without this, the
// status bar shows `tokens: 0` for the entire session regardless of how
// many LLM calls fire — the bridge never converted EventTokenUsage into
// a tea.Msg, so the Update loop never received it.
func TestTokenUsageMsg_AccumulatesTotalTokens(t *testing.T) {
	m := Model{width: 80, spinner: spinner.New()}

	// Two successive LLM calls report their usage.
	m, _ = m.applyTokenUsage(TokenUsageMsg{Input: 100, Output: 50, Total: 150})
	m, _ = m.applyTokenUsage(TokenUsageMsg{Input: 80, Output: 40, Total: 120})

	if m.totalTokens != 270 {
		t.Errorf("expected totalTokens=270 after two TokenUsageMsg, got %d", m.totalTokens)
	}
	s := m.statusBar()
	if !strings.Contains(s, "tokens: 270") {
		t.Errorf("status bar should reflect accumulated tokens, got %q", s)
	}
}
