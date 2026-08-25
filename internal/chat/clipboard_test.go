package chat

import "testing"

func TestOSC52Sequence(t *testing.T) {
	got := osc52Sequence("hello")
	want := "\x1b]52;c;aGVsbG8=\x07"
	if got != want {
		t.Errorf("osc52Sequence(%q) = %q, want %q", "hello", got, want)
	}
}

func TestOSC52SequenceEmpty(t *testing.T) {
	got := osc52Sequence("")
	want := "\x1b]52;c;\x07"
	if got != want {
		t.Errorf("osc52Sequence(\"\") = %q, want %q", got, want)
	}
}

func TestCopySelectedReply_DefaultsToLatest(t *testing.T) {
	m := Model{copySel: -1, blocks: []ContentBlock{
		{Type: BlockUser, Text: "q1"},
		{Type: BlockAssistant, Text: "first answer"},
		{Type: BlockUser, Text: "q2"},
		{Type: BlockAssistant, Text: "second answer"},
	}}
	updated, cmd := m.copySelectedReply()
	got := updated.(Model)
	if got.notice != "copied reply 2/2 to clipboard" {
		t.Errorf("notice = %q, want %q", got.notice, "copied reply 2/2 to clipboard")
	}
	if cmd == nil {
		t.Error("expected a non-nil Cmd (copy + clear-notice)")
	}
}

func TestCopySelectedReply_UsesSelection(t *testing.T) {
	m := Model{copySel: 0, blocks: []ContentBlock{
		{Type: BlockAssistant, Text: "first answer"},
		{Type: BlockAssistant, Text: "second answer"},
	}}
	updated, _ := m.copySelectedReply()
	if got := updated.(Model); got.notice != "copied reply 1/2 to clipboard" {
		t.Errorf("notice = %q, want copied reply 1/2", got.notice)
	}
}

func TestCopySelectedReply_NothingToCopy(t *testing.T) {
	m := Model{copySel: -1}
	updated, cmd := m.copySelectedReply()
	if got := updated.(Model); got.notice != "nothing to copy yet" {
		t.Errorf("notice = %q, want %q", got.notice, "nothing to copy yet")
	}
	if cmd == nil {
		t.Error("expected a non-nil Cmd (clear-notice)")
	}
}

func TestStepCopySelection_CyclesBackAndWraps(t *testing.T) {
	m := Model{copySel: -1, blocks: []ContentBlock{
		{Type: BlockAssistant, Text: "a"},
		{Type: BlockAssistant, Text: "b"},
		{Type: BlockAssistant, Text: "c"},
	}}
	for _, want := range []int{2, 1, 0, 2} { // first→latest, older, older, wrap
		updated, _ := m.stepCopySelection()
		m = updated.(Model)
		if m.copySel != want {
			t.Fatalf("stepCopySelection: copySel = %d, want %d", m.copySel, want)
		}
	}
}

func TestStepCopySelection_NoReplies(t *testing.T) {
	m := Model{copySel: -1}
	updated, cmd := m.stepCopySelection()
	if got := updated.(Model); got.notice != "no replies to copy yet" {
		t.Errorf("notice = %q, want %q", got.notice, "no replies to copy yet")
	}
	if cmd == nil {
		t.Error("expected a non-nil Cmd (clear-notice)")
	}
}

func TestAssistantReplyIndices_SkipsEmpty(t *testing.T) {
	m := Model{blocks: []ContentBlock{
		{Type: BlockUser, Text: "q"},
		{Type: BlockAssistant, Text: "real"},
		{Type: BlockAssistant, Text: "   "}, // whitespace-only — skipped
		{Type: BlockAssistant, Text: ""},    // empty — skipped
		{Type: BlockAssistant, Text: "real2"},
	}}
	idxs := m.assistantReplyIndices()
	if len(idxs) != 2 || idxs[0] != 1 || idxs[1] != 4 {
		t.Errorf("assistantReplyIndices = %v, want [1 4]", idxs)
	}
}
