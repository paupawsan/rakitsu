package chat

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// sideThreadModel puts a side thread on screen while the main conversation is
// mid-turn — the spec's own "fluid" scenario, and the state both C1 repros
// need.
func sideThreadModel(t *testing.T) Model {
	t.Helper()
	m := threadModel(t)
	m.agentName = "Coordinator"
	m.spawnParents = map[string]string{}
	m.blocks = []ContentBlock{
		{Type: BlockUser, Text: "main question"},
		{Type: BlockAssistant, Text: "partial main"},
	}
	m.generating = true
	m.genToken = 1
	return m.switchThread("Reviewer")
}

// TestSideThreadToolCallDoesNotPolluteMainTranscript is C1 failure scenario A,
// with no preconditions beyond "the side agent uses a tool" — the headline use
// case. Roster.Send runs the rebuilt agent through the same RunWithHistory
// path as the main run and it emits TOOL_CALL_START/END on the shared bus, so
// with no source filter a tool block for a call main never made was inserted
// into the main conversation's transcript.
func TestSideThreadToolCallDoesNotPolluteMainTranscript(t *testing.T) {
	m := sideThreadModel(t)
	next, _ := m.submitToAgent("Reviewer", "why did you skip the API docs?")
	m = next.(Model)

	m = step(t, m, ToolCallStartMsg{ID: "call_1", Name: "fs_read", AgentName: "Reviewer", Directed: true})
	m = step(t, m, ToolCallEndMsg{ID: "call_1", Name: "fs_read", Output: "the README", AgentName: "Reviewer", Directed: true})

	for _, b := range m.threads[""].blocks {
		if b.Type == BlockTool {
			t.Fatalf("a side thread's tool call was injected into the main transcript: %+v", b)
		}
	}
}

// TestSideThreadTokenChunkDoesNotAppendToMainAnswer is C1 failure scenario B.
// The side agent's streamed tokens were appended to the main answer's
// assistant block; AgentDoneMsg then reads lastAssistantText and appends it to
// m.history, so text the host never produced was written into the host's LLM
// conversation and re-fed to the model on the next turn.
func TestSideThreadTokenChunkDoesNotAppendToMainAnswer(t *testing.T) {
	m := sideThreadModel(t)
	next, _ := m.submitToAgent("Reviewer", "why did you skip the API docs?")
	m = next.(Model)

	m = step(t, m, TokenChunkMsg{Text: " SIDE-THREAD-TEXT", AgentName: "Reviewer", Directed: true})
	m = step(t, m, ReasoningChunkMsg{Text: " SIDE-THREAD-COT", AgentName: "Reviewer", Directed: true})

	for _, b := range m.threads[""].blocks {
		if strings.Contains(b.Text, "SIDE-THREAD") {
			t.Fatalf("side-thread output landed in the main conversation: %+v", b)
		}
	}
}

// TestMainRunSpawnedChildStillRendersInMain is the guard against fixing C1 the
// wrong way. The discriminator must be "did this event come from a directed
// Send", never "the agent name is not the host": a child the MAIN run spawned
// legitimately renders in main even while the user reads a side thread. That
// live subagent visibility must survive this fix.
func TestMainRunSpawnedChildStillRendersInMain(t *testing.T) {
	m := sideThreadModel(t)
	// Untagged events — Researcher-1 here is the main run's child.
	m = step(t, m, AgentStartMsg{Name: "Researcher-1", Parent: "Coordinator", Model: "m1"})
	m = step(t, m, ToolCallStartMsg{ID: "call_9", Name: "fs_read", AgentName: "Researcher-1"})
	m = step(t, m, TokenChunkMsg{Text: " from the child", AgentName: "Researcher-1"})
	m = step(t, m, AgentEndMsg{Name: "Researcher-1", Status: "success", Tokens: 10})

	var sawSubagent, sawTool, sawText bool
	for _, b := range m.threads[""].blocks {
		switch {
		case b.Type == BlockSubagent && b.AgentName == "Researcher-1":
			sawSubagent = true
			if !b.SubDone {
				t.Error("the main run's spawned child never got its AGENT_END")
			}
		case b.Type == BlockTool && b.ToolName == "fs_read":
			sawTool = true
		case b.Type == BlockAssistant && strings.Contains(b.Text, "from the child"):
			sawText = true
		}
	}
	if !sawSubagent || !sawTool || !sawText {
		t.Fatalf("a main-run spawned child must still render in main (subagent=%v tool=%v text=%v): %+v",
			sawSubagent, sawTool, sawText, m.threads[""].blocks)
	}
}

// TestMainRunEventsRenderWhileASideChatWithTheSameAgentRuns is the mirror
// image of the two tests above, and the reason the filter cannot be keyed on
// agent name. Config agents are addressable from the moment BuildRunner calls
// AddConfig, and orchestrator delegation never marks them running, so a user
// can open Reviewer's side thread while the MAIN run is delegating to
// Reviewer — claim accepts it. A name-keyed filter then swallows the main
// run's own tool rows and streamed text for the whole side-chat turn, with
// nothing on screen saying so.
func TestMainRunEventsRenderWhileASideChatWithTheSameAgentRuns(t *testing.T) {
	m := sideThreadModel(t)
	next, _ := m.submitToAgent("Reviewer", "why did you skip the API docs?")
	m = next.(Model)

	// Emitted by the MAIN run's delegated worker, not by the side chat.
	m = step(t, m, ToolCallStartMsg{ID: "call_main", Name: "fs_read", AgentName: "Reviewer"})
	m = step(t, m, ToolCallEndMsg{ID: "call_main", Name: "fs_read", Output: "the spec", AgentName: "Reviewer"})
	m = step(t, m, TokenChunkMsg{Text: " MAIN-RUN-TEXT", AgentName: "Reviewer"})

	var sawTool, sawText bool
	for _, b := range m.threads[""].blocks {
		if b.Type == BlockTool && b.ToolID == "call_main" {
			sawTool = true
		}
		if strings.Contains(b.Text, "MAIN-RUN-TEXT") {
			sawText = true
		}
	}
	if !sawTool || !sawText {
		t.Fatalf("a side chat blinded the main transcript (tool=%v text=%v): %+v",
			sawTool, sawText, m.threads[""].blocks)
	}
}

// TestCancelledSideChatDoesNotKeepSuppressingMainEvents pins the residual of
// the same root cause: Ctrl+C clears `generating` immediately, but the Send is
// still unwinding its HTTP call and its AgentChatDoneMsg has not arrived. Any
// discriminator held in Model state stays armed across that whole window — a
// full bubbletea round trip, not microseconds. An origin-tagged event has no
// such window.
func TestCancelledSideChatDoesNotKeepSuppressingMainEvents(t *testing.T) {
	m := sideThreadModel(t)
	next, _ := m.submitToAgent("Reviewer", "why did you skip the API docs?")
	m = next.(Model)

	out, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = out.(Model)

	m = step(t, m, ToolCallStartMsg{ID: "call_after_cancel", Name: "fs_read", AgentName: "Reviewer"})
	for _, b := range m.threads[""].blocks {
		if b.Type == BlockTool && b.ToolID == "call_after_cancel" {
			return
		}
	}
	t.Fatalf("a cancelled side chat kept suppressing the main run's rows: %+v", m.threads[""].blocks)
}

// TestSideThreadToolEndCannotCompleteAMainRunToolBlock pins the ToolCallEndMsg
// guard on its own. Tool-call IDs are not globally unique: gemini synthesizes
// call_<name>_<index> (internal/llm/gemini/provider.go) and the inline
// tool-call format synthesizes content_tc_<i>, so two agents' first fs_read
// share an id. With only the START guarded, the sibling test passes for the
// wrong reason — completeToolBlock finds no block and no-ops — and removing
// the END guard leaves the whole suite green. Here the main run has an OPEN
// block with the id the side chat reuses, so the END has something to corrupt.
func TestSideThreadToolEndCannotCompleteAMainRunToolBlock(t *testing.T) {
	m := sideThreadModel(t)

	// The main run opens call_1 and it is still running.
	m = step(t, m, ToolCallStartMsg{ID: "call_1", Name: "fs_read", AgentName: "Coordinator"})
	// The side chat's own fs_read gets the same synthesized id and finishes first.
	m = step(t, m, ToolCallEndMsg{ID: "call_1", Name: "fs_read", Output: "SIDE-THREAD-OUTPUT", AgentName: "Reviewer", Directed: true})

	var found bool
	for _, b := range m.threads[""].blocks {
		if b.Type != BlockTool || b.ToolID != "call_1" {
			continue
		}
		found = true
		if b.ToolDone || b.Text == "SIDE-THREAD-OUTPUT" {
			t.Fatalf("a side chat's TOOL_CALL_END closed the main run's open tool block with its own output: %+v", b)
		}
	}
	if !found {
		t.Fatal("the main run's tool block is missing — the test proved nothing")
	}
}
