package chat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// step drives one Update tick and returns the concrete Model.
func step(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

// TestUpdate_TurnOrdersBlocks is an integration test for the three chat
// fixes wired together. It replays a config-10-shaped turn through Update:
// the ChatHost reasons, delegates to a sub-agent, the sub-agent reasons and
// runs a tool, then the answer streams. It asserts the resulting block
// stream nests sub-agent activity, keeps the answer last, and captures
// reasoning chunks (reasoning visibility).
func TestUpdate_TurnOrdersBlocks(t *testing.T) {
	// ready:false makes updateViewport a no-op so no TTY/renderer is needed.
	m := Model{
		agentName:  "ChatHost",
		generating: true,
		blocks: []ContentBlock{
			{Type: BlockUser, Text: "what Go packages exist?"},
			{Type: BlockAssistant},
		},
	}

	m = step(t, m, ReasoningChunkMsg{Text: "route this", AgentName: "ChatHost"})
	m = step(t, m, ToolCallStartMsg{ID: "d1", Name: "delegate_to_backendauditor", AgentName: "ChatHost"})
	m = step(t, m, ReasoningChunkMsg{Text: "scan internal/", AgentName: "BackendAuditor"})
	m = step(t, m, ToolCallStartMsg{ID: "t1", Name: "list_files", AgentName: "BackendAuditor"})
	m = step(t, m, ToolCallEndMsg{ID: "t1", Name: "list_files", Output: "agent\nllm\ntools", Duration: 5})
	m = step(t, m, TokenChunkMsg{Text: "The packages are..."})

	// Expected stream: user, reasoning(ChatHost), tool(delegate),
	// reasoning(BackendAuditor), tool(list_files), assistant.
	want := []BlockType{
		BlockUser, BlockReasoning, BlockTool, BlockReasoning, BlockTool, BlockAssistant,
	}
	got := blockTypes(m.blocks)
	if len(got) != len(want) {
		t.Fatalf("block stream = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("block[%d] = %v, want %v (full: %v)", i, got[i], want[i], got)
		}
	}

	// The streamed answer must be the last block.
	last := m.blocks[len(m.blocks)-1]
	if last.Type != BlockAssistant || last.Text != "The packages are..." {
		t.Errorf("answer must be last and hold the streamed text, got %+v", last)
	}

	// The ChatHost delegate call is depth 0, the sub-agent's tool is depth 1
	// (nested).
	if m.blocks[2].Depth != 0 {
		t.Errorf("ChatHost delegate call should be depth 0, got %d", m.blocks[2].Depth)
	}
	if m.blocks[4].Depth != 1 || m.blocks[4].AgentName != "BackendAuditor" {
		t.Errorf("sub-agent tool should be depth 1 / BackendAuditor, got depth=%d agent=%q",
			m.blocks[4].Depth, m.blocks[4].AgentName)
	}

	// Reasoning visibility: the sub-agent's reasoning was captured, nested.
	if m.blocks[3].Type != BlockReasoning || m.blocks[3].Depth != 1 {
		t.Errorf("sub-agent reasoning should be a depth-1 BlockReasoning, got %+v", m.blocks[3])
	}
	if m.blocks[3].Text != "scan internal/" {
		t.Errorf("sub-agent reasoning text = %q, want %q", m.blocks[3].Text, "scan internal/")
	}

	// The completed tool carries its output.
	if !m.blocks[4].ToolDone || m.blocks[4].Text != "agent\nllm\ntools" {
		t.Errorf("tool block should be completed with output, got %+v", m.blocks[4])
	}
}

// TestInsertBeforeAssistant_KeepsAnswerLast is the core ordering fix:
// tool and reasoning blocks must land *before* the turn's trailing assistant
// block so the streamed answer renders below the tools it depended on —
// never buried above them.
func TestInsertBeforeAssistant_KeepsAnswerLast(t *testing.T) {
	m := &Model{blocks: []ContentBlock{
		{Type: BlockUser, Text: "q"},
		{Type: BlockAssistant},
	}}

	m.insertBeforeAssistant(ContentBlock{Type: BlockTool, ToolName: "t1"})
	m.insertBeforeAssistant(ContentBlock{Type: BlockTool, ToolName: "t2"})

	if got := len(m.blocks); got != 4 {
		t.Fatalf("expected 4 blocks, got %d", got)
	}
	if m.blocks[len(m.blocks)-1].Type != BlockAssistant {
		t.Errorf("assistant block must remain last; order = %v", blockTypes(m.blocks))
	}
	if m.blocks[1].ToolName != "t1" || m.blocks[2].ToolName != "t2" {
		t.Errorf("tools should keep execution order before the answer, got %v", blockTypes(m.blocks))
	}
}

// TestInsertBeforeAssistant_NoAssistantBlock — before the assistant block
// exists, a block appends at the end of the turn.
func TestInsertBeforeAssistant_NoAssistantBlock(t *testing.T) {
	m := &Model{blocks: []ContentBlock{{Type: BlockUser, Text: "q"}}}
	m.insertBeforeAssistant(ContentBlock{Type: BlockTool, ToolName: "t1"})
	if len(m.blocks) != 2 || m.blocks[1].ToolName != "t1" {
		t.Errorf("tool should append when no assistant block exists, got %v", blockTypes(m.blocks))
	}
}

// TestAgentDepth verifies the nesting signal: the root agent is depth 0, any
// delegated sub-agent is depth 1.
func TestAgentDepth(t *testing.T) {
	m := Model{agentName: "ChatHost"}
	cases := []struct {
		agent string
		want  int
	}{
		{"ChatHost", 0}, // root agent
		{"", 0},         // unattributed → treated as root
		{"Researcher", 1},
	}
	for _, c := range cases {
		if got := m.agentDepth(c.agent); got != c.want {
			t.Errorf("agentDepth(%q) = %d, want %d", c.agent, got, c.want)
		}
	}
}

// TestAppendReasoning_ReusesOpenSegment — consecutive reasoning deltas
// accumulate into a single BlockReasoning block until a tool call seals it.
func TestAppendReasoning_ReusesOpenSegment(t *testing.T) {
	m := &Model{blocks: []ContentBlock{
		{Type: BlockUser, Text: "q"},
		{Type: BlockAssistant},
	}}

	m.appendReasoning("think-1 ", "")
	m.appendReasoning("think-2", "")

	reasoning := blocksOfType(m.blocks, BlockReasoning)
	if len(reasoning) != 1 {
		t.Fatalf("expected 1 reasoning block while segment is open, got %d", len(reasoning))
	}
	if reasoning[0].Text != "think-1 think-2" {
		t.Errorf("reasoning deltas should accumulate, got %q", reasoning[0].Text)
	}

	// A tool call seals the segment; the next delta starts a fresh block.
	m.reasoningSealed = true
	m.appendReasoning("post-tool thought", "")
	if got := len(blocksOfType(m.blocks, BlockReasoning)); got != 2 {
		t.Errorf("a sealed segment should start a new reasoning block, got %d", got)
	}
}

// TestAppendReasoning_OrdersBeforeAnswer — reasoning blocks also sit before
// the trailing assistant block.
func TestAppendReasoning_OrdersBeforeAnswer(t *testing.T) {
	m := &Model{blocks: []ContentBlock{
		{Type: BlockUser, Text: "q"},
		{Type: BlockAssistant},
	}}
	m.appendReasoning("thinking", "")
	if m.blocks[len(m.blocks)-1].Type != BlockAssistant {
		t.Errorf("assistant block must stay last after reasoning, got %v", blockTypes(m.blocks))
	}
}

// TestIsThinking — true while reasoning streams with no answer yet, false
// once visible answer text appears.
func TestIsThinking(t *testing.T) {
	m := Model{blocks: []ContentBlock{
		{Type: BlockUser, Text: "q"},
		{Type: BlockReasoning, Text: "thinking..."},
		{Type: BlockAssistant, Text: ""},
	}}
	if !m.isThinking() {
		t.Errorf("should report thinking while reasoning streams with no answer")
	}

	m.blocks[2].Text = "here is the answer"
	if m.isThinking() {
		t.Errorf("should not report thinking once answer text exists")
	}
}

func blockTypes(blocks []ContentBlock) []BlockType {
	out := make([]BlockType, len(blocks))
	for i, b := range blocks {
		out[i] = b.Type
	}
	return out
}

func blocksOfType(blocks []ContentBlock, t BlockType) []ContentBlock {
	var out []ContentBlock
	for _, b := range blocks {
		if b.Type == t {
			out = append(out, b)
		}
	}
	return out
}

// readyModel returns a Model with a viewport + glamour renderer set up, as if
// the first tea.WindowSizeMsg had arrived — needed to exercise updateViewport.
func readyModel(t *testing.T) Model {
	t.Helper()
	m := Model{width: 80, height: 24, historyIdx: -1}
	m.textarea = textarea.New()
	return m.handleResize()
}

// TestUpdateViewport_Spans verifies the line-span table is contiguous and
// every non-empty block occupies at least one line — this is the foundation
// for mouse click hit-testing, and also catches per-block glamour breakage.
func TestUpdateViewport_Spans(t *testing.T) {
	m := readyModel(t)
	m.blocks = []ContentBlock{
		{Type: BlockUser, Text: "hello"},
		{Type: BlockReasoning, Text: "thinking about it"},
		{Type: BlockTool, ToolName: "search", ToolDone: true, Text: "result", Duration: 5},
		{Type: BlockAssistant, Text: "the answer"},
	}
	m.updateViewport()

	if len(m.spans) != len(m.blocks) {
		t.Fatalf("expected %d spans, got %d", len(m.blocks), len(m.spans))
	}
	for i := 1; i < len(m.spans); i++ {
		if m.spans[i].start != m.spans[i-1].start+m.spans[i-1].count {
			t.Errorf("span %d not contiguous: %+v after %+v", i, m.spans[i], m.spans[i-1])
		}
	}
	for i, s := range m.spans {
		if s.count < 1 {
			t.Errorf("block %d (%v) span count = %d, want >=1", i, m.blocks[i].Type, s.count)
		}
	}
}

// TestBlockAt maps rendered-content lines back to block indices, skipping
// zero-height (empty) blocks.
func TestBlockAt(t *testing.T) {
	m := Model{spans: []blockSpan{{start: 0, count: 2}, {start: 2, count: 0}, {start: 2, count: 3}}}
	cases := []struct{ line, want int }{
		{0, 0}, {1, 0}, {2, 2}, {4, 2}, {5, -1}, {-1, -1},
	}
	for _, c := range cases {
		if got := m.blockAt(c.line); got != c.want {
			t.Errorf("blockAt(%d) = %d, want %d", c.line, got, c.want)
		}
	}
}

// TestHandleClick_TogglesBlock — a click inside a reasoning block's screen
// span flips its collapsed state.
func TestHandleClick_TogglesBlock(t *testing.T) {
	m := readyModel(t)
	m.blocks = []ContentBlock{
		{Type: BlockUser, Text: "q"},
		{Type: BlockReasoning, Text: "line1\nline2\nline3"},
		{Type: BlockAssistant, Text: "answer"},
	}
	m.updateViewport()

	// Screen Y for the reasoning block's first line: title is row 0, the
	// sticky user-prompt header occupies stickyHeaderHeight rows, the
	// viewport starts below them, content line = YOffset + (Y - chrome).
	y := 1 + stickyHeaderHeight + (m.spans[1].start - m.viewport.YOffset)
	if m.blocks[1].Collapsed {
		t.Fatal("precondition: reasoning should start expanded")
	}
	m.handleClick(y)
	if !m.blocks[1].Collapsed {
		t.Errorf("click inside reasoning block should collapse it")
	}
}

// TestHandleClick_OutsideViewport — a click on the title/textarea/status
// rows toggles nothing.
func TestHandleClick_OutsideViewport(t *testing.T) {
	m := readyModel(t)
	m.blocks = []ContentBlock{
		{Type: BlockReasoning, Text: "thinking"},
	}
	m.updateViewport()
	m.handleClick(0) // title row
	if m.blocks[0].Collapsed {
		t.Errorf("click on the title row must not toggle a block")
	}
}

// TestCollapseOpenReasoning collapses only the most recent reasoning segment
// of the current turn, leaving older (already-collapsed) and earlier-turn
// blocks alone.
func TestCollapseOpenReasoning(t *testing.T) {
	m := &Model{blocks: []ContentBlock{
		{Type: BlockUser},
		{Type: BlockReasoning, Text: "old", Collapsed: false},
		{Type: BlockTool},
		{Type: BlockReasoning, Text: "live", Collapsed: false},
		{Type: BlockAssistant},
	}}
	m.collapseOpenReasoning()
	if !m.blocks[3].Collapsed {
		t.Errorf("the live reasoning segment should be collapsed")
	}
	if m.blocks[1].Collapsed {
		t.Errorf("an earlier reasoning segment should be left untouched")
	}
}

// TestCollapseAllReasoning collapses every reasoning block (turn-end tidy-up).
func TestCollapseAllReasoning(t *testing.T) {
	m := &Model{blocks: []ContentBlock{
		{Type: BlockReasoning, Text: "a"},
		{Type: BlockTool},
		{Type: BlockReasoning, Text: "b"},
		{Type: BlockAssistant},
	}}
	m.collapseAllReasoning()
	for i, b := range m.blocks {
		if b.Type == BlockReasoning && !b.Collapsed {
			t.Errorf("reasoning block %d should be collapsed", i)
		}
	}
}

// TestHistoryRecall walks Up/Down through submitted messages and confirms the
// unsent draft is preserved and restored on stepping past the newest entry.
func TestHistoryRecall(t *testing.T) {
	m := &Model{historyIdx: -1, inputHistory: []string{"first", "second", "third"}}
	m.textarea = textarea.New()
	m.textarea.SetValue("draft")

	m.recallPrev() // → third
	if got := m.textarea.Value(); got != "third" {
		t.Fatalf("first recallPrev = %q, want %q", got, "third")
	}
	m.recallPrev() // → second
	m.recallPrev() // → first
	m.recallPrev() // clamped at oldest
	if got := m.textarea.Value(); got != "first" {
		t.Fatalf("recallPrev should clamp at the oldest entry, got %q", got)
	}
	m.recallNext() // → second
	m.recallNext() // → third
	m.recallNext() // past newest → restore draft
	if got := m.textarea.Value(); got != "draft" {
		t.Errorf("stepping past the newest entry should restore the draft, got %q", got)
	}
	if m.historyIdx != -1 {
		t.Errorf("historyIdx should reset to -1 after restoring the draft, got %d", m.historyIdx)
	}
}

// lineIndexOf returns the index of the first line in s containing sub, or -1.
func lineIndexOf(s, sub string) int {
	for i, ln := range strings.Split(s, "\n") {
		if strings.Contains(ln, sub) {
			return i
		}
	}
	return -1
}

// TestRenderTool_OutputLinesSurviveGlamour — multi-line tool output must
// render on separate lines, not merge into one flowing paragraph (the
// markdown blockquote line-merge bug).
func TestRenderTool_OutputLinesSurviveGlamour(t *testing.T) {
	m := readyModel(t)
	m.blocks = []ContentBlock{
		{Type: BlockTool, ToolName: "ls", ToolDone: true, Text: "AAAA/\nBBBB/\nCCCC/"},
	}
	m.updateViewport()
	content := m.viewport.View()
	a, b, c := lineIndexOf(content, "AAAA"), lineIndexOf(content, "BBBB"), lineIndexOf(content, "CCCC")
	if a < 0 || b < 0 || c < 0 {
		t.Fatalf("tool output lines missing from render: AAAA@%d BBBB@%d CCCC@%d", a, b, c)
	}
	if a == b || b == c {
		t.Errorf("tool output lines merged onto one rendered line: AAAA@%d BBBB@%d CCCC@%d", a, b, c)
	}
}

// TestStickyHeaderPicksMostRecentUserAboveOffset — when scrolled into the
// middle of a turn, the sticky header should resolve to the user prompt that
// started that turn, not an earlier or later one.
func TestStickyHeaderPicksMostRecentUserAboveOffset(t *testing.T) {
	m := readyModel(t)
	// Two turns. The assistant response of each is padded with system blocks
	// so the rendered spans cover many viewport lines — gives us room to
	// scroll into a specific turn.
	m.blocks = []ContentBlock{
		{Type: BlockUser, Text: "first question"},
	}
	for i := 0; i < 20; i++ {
		m.blocks = append(m.blocks, ContentBlock{Type: BlockSystem, Text: fmt.Sprintf("turn1 line %d", i)})
	}
	secondUserIdx := len(m.blocks)
	m.blocks = append(m.blocks, ContentBlock{Type: BlockUser, Text: "second question"})
	for i := 0; i < 20; i++ {
		m.blocks = append(m.blocks, ContentBlock{Type: BlockSystem, Text: fmt.Sprintf("turn2 line %d", i)})
	}
	m.updateViewport()

	// Top of the chat — nothing to pin.
	m.viewport.SetYOffset(0)
	if got := m.stickyUserBlockIdx(); got != -1 {
		t.Errorf("at YOffset=0 expected stickyUserBlockIdx=-1, got %d", got)
	}

	// Scroll just past the first user prompt — pin the first user.
	firstUserSpan := m.spans[0]
	m.viewport.SetYOffset(firstUserSpan.start + firstUserSpan.count + 1)
	if got := m.stickyUserBlockIdx(); got != 0 {
		t.Errorf("scrolled into turn 1 expected stickyUserBlockIdx=0, got %d", got)
	}

	// Scroll into turn 2 — pin the second user.
	secondUserSpan := m.spans[secondUserIdx]
	m.viewport.SetYOffset(secondUserSpan.start + secondUserSpan.count + 1)
	if got := m.stickyUserBlockIdx(); got != secondUserIdx {
		t.Errorf("scrolled into turn 2 expected stickyUserBlockIdx=%d, got %d", secondUserIdx, got)
	}
}

// TestRenderStickyHeader_BlankWhenAtTop — at YOffset=0 the header line is a
// blank-padded row of the reserved height. This keeps block line spans stable
// across scroll transitions (no reflow when the sticky appears/disappears).
func TestRenderStickyHeader_BlankWhenAtTop(t *testing.T) {
	m := readyModel(t)
	m.blocks = []ContentBlock{
		{Type: BlockUser, Text: "anything"},
		{Type: BlockAssistant, Text: "answer"},
	}
	m.updateViewport()
	m.viewport.SetYOffset(0)

	got := m.renderStickyHeader()
	if strings.TrimSpace(got) != "" {
		t.Errorf("at YOffset=0 expected blank header, got %q", got)
	}
}

// TestRenderStickyHeader_TruncatesLongPrompt — a multi-line user prompt is
// reduced to its first line, then truncated with an ellipsis to fit the
// viewport width.
func TestRenderStickyHeader_TruncatesLongPrompt(t *testing.T) {
	m := readyModel(t)
	longPrompt := "a very long single-line user prompt that should be truncated because it exceeds the chat viewport width by quite a bit indeed"
	m.blocks = []ContentBlock{
		{Type: BlockUser, Text: longPrompt + "\nsecond line that must not appear"},
		{Type: BlockAssistant, Text: "answer"},
	}
	for i := 0; i < 10; i++ {
		m.blocks = append(m.blocks, ContentBlock{Type: BlockSystem, Text: fmt.Sprintf("filler %d", i)})
	}
	m.updateViewport()
	m.viewport.SetYOffset(m.spans[0].start + m.spans[0].count + 1)

	got := m.renderStickyHeader()
	if strings.Contains(got, "second line") {
		t.Errorf("sticky header leaked second line of multiline prompt: %q", got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("sticky header for overlong prompt should contain ellipsis, got %q", got)
	}
}

// TestUpdateViewport_AutoFollow — a user who scrolled up to read is not
// yanked back to the bottom by a re-render.
func TestUpdateViewport_AutoFollow(t *testing.T) {
	m := readyModel(t)
	for i := 0; i < 40; i++ {
		m.blocks = append(m.blocks, ContentBlock{Type: BlockSystem, Text: fmt.Sprintf("line %d", i)})
	}
	m.updateViewport()

	// At the bottom → a re-render keeps us at the bottom.
	m.viewport.GotoBottom()
	m.updateViewport()
	if !m.viewport.AtBottom() {
		t.Errorf("a viewport at the bottom should stay at the bottom after re-render")
	}

	// Scrolled up → a re-render leaves the scroll position alone.
	m.viewport.SetYOffset(0)
	m.updateViewport()
	if m.viewport.YOffset != 0 {
		t.Errorf("a scrolled-up viewport should stay put, got YOffset=%d", m.viewport.YOffset)
	}
}

func TestAgentUsage_AccumulatesAcrossTurns(t *testing.T) {
	m := Model{width: 80, spinner: spinner.New(), spawnParents: map[string]string{}, agentUsage: map[string]*agentUsageSnapshot{}}
	m.recordAgentUsageStart(AgentStartMsg{Name: "Researcher", Model: "gemini-3.5-flash-lite", Provider: "litellm"})
	m, _ = m.updateAgentUsageEnd(AgentEndMsg{Name: "Researcher", Status: "success", Tokens: 100, Cost: 0.01, MaxTokens: 1000, PricingKnown: true, Iterations: 2})
	m, _ = m.updateAgentUsageEnd(AgentEndMsg{Name: "Researcher", Status: "success", Tokens: 250, Cost: 0.025, MaxTokens: 1000, PricingKnown: true, Iterations: 3})

	snap, ok := m.agentUsage["Researcher"]
	if !ok {
		t.Fatalf("agentUsage has no entry for Researcher")
	}
	if snap.Model != "gemini-3.5-flash-lite" || snap.Provider != "litellm" {
		t.Errorf("Model/Provider = %q/%q, want gemini-3.5-flash-lite/litellm", snap.Model, snap.Provider)
	}
	// Tokens/Cost overwrite (TokenGuard is already cumulative per agent instance).
	if snap.Tokens != 250 {
		t.Errorf("Tokens = %d, want 250 (overwrite, not sum)", snap.Tokens)
	}
	if snap.Cost != 0.025 {
		t.Errorf("Cost = %v, want 0.025 (overwrite, not sum)", snap.Cost)
	}
	// Iterations/Turns accumulate — each AgentEndMsg is a distinct call.
	if snap.TotalIterations != 5 {
		t.Errorf("TotalIterations = %d, want 5 (2+3, summed)", snap.TotalIterations)
	}
	if snap.Turns != 2 {
		t.Errorf("Turns = %d, want 2", snap.Turns)
	}
}

func TestAgentUsage_EndWithoutPriorStart_StillRecorded(t *testing.T) {
	// bridge.go/model.go never filter AgentEndMsg by parent, but this guards
	// against a future refactor accidentally requiring a prior Start.
	m := Model{width: 80, spinner: spinner.New(), spawnParents: map[string]string{}, agentUsage: map[string]*agentUsageSnapshot{}}
	m, _ = m.updateAgentUsageEnd(AgentEndMsg{Name: "Ghost", Status: "success", Tokens: 10})
	if _, ok := m.agentUsage["Ghost"]; !ok {
		t.Errorf("expected an entry to be created even without a preceding AgentStartMsg")
	}
}

// TestAgentUsage_RootAgentTrackedThroughUpdate verifies the critical
// behavior that recordAgentUsageStart runs unconditionally BEFORE the
// if msg.Parent == "" early-return in case AgentStartMsg. Without this
// ordering, a future refactor moving recordAgentUsageStart below the
// early-return would silently break root-agent usage tracking.
func TestAgentUsage_RootAgentTrackedThroughUpdate(t *testing.T) {
	m := Model{
		width:        80,
		spinner:      spinner.New(),
		spawnParents: map[string]string{},
		agentUsage:   map[string]*agentUsageSnapshot{},
		agentName:    "RootAgent",
	}

	// Send AgentStartMsg through the real Update() dispatch. Parent="" is
	// the critical case — it triggers the early-return that suppresses UI
	// rendering (root agent AGENT_START is noise), but usage tracking must
	// still run unconditionally.
	updated, _ := m.Update(AgentStartMsg{
		Name:     "RootAgent",
		Model:    "gpt-4o-mini",
		Provider: "openai",
		Parent:   "",
	})
	m2 := updated.(Model)

	// Assert that agentUsage["RootAgent"] was populated by recordAgentUsageStart
	// despite the Parent="" early-return.
	snap, ok := m2.agentUsage["RootAgent"]
	if !ok {
		t.Fatalf("agentUsage has no entry for RootAgent after AgentStartMsg with Parent=\"\"")
	}
	if snap.Model != "gpt-4o-mini" {
		t.Errorf("Model = %q, want gpt-4o-mini", snap.Model)
	}
	if snap.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", snap.Provider)
	}

	// Also verify that AgentEndMsg accumulates tokens through the real dispatch.
	updated2, _ := m2.Update(AgentEndMsg{
		Name:   "RootAgent",
		Status: "success",
		Tokens: 42,
	})
	m3 := updated2.(Model)

	snapEnd, ok := m3.agentUsage["RootAgent"]
	if !ok {
		t.Fatalf("agentUsage lost entry for RootAgent after AgentEndMsg")
	}
	if snapEnd.Tokens != 42 {
		t.Errorf("Tokens = %d, want 42", snapEnd.Tokens)
	}
}

func TestCtrlU_OpensUsagePopup(t *testing.T) {
	m := Model{width: 80, height: 24, ready: true, spinner: spinner.New(), agentUsage: map[string]*agentUsageSnapshot{}}
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlU})
	m2 := updated.(Model)
	if m2.activePopup != popupUsage {
		t.Errorf("activePopup = %v, want popupUsage", m2.activePopup)
	}
}

func TestPopupEsc_Closes(t *testing.T) {
	m := Model{width: 80, height: 24, ready: true, spinner: spinner.New(), activePopup: popupUsage}
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := updated.(Model)
	if m2.activePopup != popupNone {
		t.Errorf("activePopup = %v, want popupNone after Esc", m2.activePopup)
	}
}

func TestPopupD_TogglesDetail(t *testing.T) {
	m := Model{width: 80, height: 24, ready: true, spinner: spinner.New(), activePopup: popupUsage}
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m2 := updated.(Model)
	if !m2.popupDetail {
		t.Errorf("popupDetail = false, want true after pressing d")
	}
}

func TestPopupOpen_SwallowsOtherKeys(t *testing.T) {
	m := Model{width: 80, height: 24, ready: true, spinner: spinner.New(), activePopup: popupUsage}
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m2 := updated.(Model)
	if m2.activePopup != popupUsage {
		t.Errorf("activePopup = %v, want popupUsage unchanged (only Esc closes)", m2.activePopup)
	}
}

func TestSlashUsage_OpensPopup(t *testing.T) {
	m := Model{width: 80, height: 24, ready: true, spinner: spinner.New(), agentUsage: map[string]*agentUsageSnapshot{}}
	updated, _ := m.handleSlashCommand(&SlashCommand{Name: "usage"})
	m2 := updated.(Model)
	if m2.activePopup != popupUsage {
		t.Errorf("activePopup = %v, want popupUsage after /usage", m2.activePopup)
	}
}

func TestSlashHistory_NoLongerRecognized(t *testing.T) {
	m := Model{width: 80, height: 24, ready: true, spinner: spinner.New()}
	updated, _ := m.handleSlashCommand(&SlashCommand{Name: "history"})
	m2 := updated.(Model)
	if m2.activePopup == popupUsage {
		t.Errorf("/history should not open the usage popup — it should fall through to the unknown-command path")
	}
}

func TestPopupE_ExportsMarkdown(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	m := Model{width: 80, height: 24, ready: true, spinner: spinner.New(), activePopup: popupUsage, agentUsage: map[string]*agentUsageSnapshot{}}
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m2 := updated.(Model)
	if !strings.Contains(m2.notice, "exported to") {
		t.Errorf("notice = %q, want it to confirm export", m2.notice)
	}
}

func TestSlashUsageExport_WorksWithoutPopupOpen(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	m := Model{width: 80, height: 24, ready: true, spinner: spinner.New(), agentUsage: map[string]*agentUsageSnapshot{}}
	updated, _ := m.handleSlashCommand(&SlashCommand{Name: "usage", Args: "export csv"})
	m2 := updated.(Model)
	if m2.activePopup != popupNone {
		t.Errorf("activePopup = %v, want popupNone (export via slash shouldn't open the popup)", m2.activePopup)
	}
	if !strings.Contains(m2.notice, "exported to") {
		t.Errorf("notice = %q, want it to confirm export", m2.notice)
	}
}

func TestCtrlX_OpensContextPopup(t *testing.T) {
	m := Model{width: 80, height: 24, ready: true, spinner: spinner.New(), agentUsage: map[string]*agentUsageSnapshot{}}
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlX})
	m2 := updated.(Model)
	if m2.activePopup != popupContext {
		t.Errorf("activePopup = %v, want popupContext", m2.activePopup)
	}
}

func TestSlashContext_OpensPopup(t *testing.T) {
	m := Model{width: 80, height: 24, ready: true, spinner: spinner.New(), agentUsage: map[string]*agentUsageSnapshot{}}
	updated, _ := m.handleSlashCommand(&SlashCommand{Name: "context"})
	m2 := updated.(Model)
	if m2.activePopup != popupContext {
		t.Errorf("activePopup = %v, want popupContext after /context", m2.activePopup)
	}
}

// TestAgentDoneMsg_ErrorPreservesHistoryAlternation regression-guards:
// submitQuery unconditionally appends a "user" message to m.history before
// a turn runs. The AgentDoneMsg handler only appended a matching
// "assistant" message when it had real streamed/final text — on
// msg.Err != nil, only a UI-facing BlockSystem "Error: ..." entry was
// appended, so a failed turn left m.history ending in "user" with no
// reply. The next submitQuery then appended another "user" message right
// after it, producing two consecutive user-role entries — Anthropic's
// Messages API rejects that with a 400 on the very next turn.
func TestAgentDoneMsg_ErrorPreservesHistoryAlternation(t *testing.T) {
	m := Model{spinner: spinner.New(), agentUsage: map[string]*agentUsageSnapshot{}}
	updated, _ := m.submitQuery("do the thing")
	m = updated.(Model)

	updated, _ = m.Update(AgentDoneMsg{Token: m.genToken, Err: errStub{}})
	m = updated.(Model)

	if len(m.history) != 2 {
		t.Fatalf("history = %d entries after a failed turn, want 2 (user + assistant placeholder): %+v", len(m.history), m.history)
	}
	if role := m.history[len(m.history)-1].Role; role != "assistant" {
		t.Fatalf("a failed turn left history ending in role %q — two consecutive user messages break strict alternation: %+v", role, m.history)
	}
}
