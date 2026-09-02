package acp

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSession_StartTurn_ArmsCancelAndClearsCancelled(t *testing.T) {
	sess := newSession("s1")
	sess.requestCancel() // mark cancelled with nothing in flight yet
	if !sess.wasCancelled() {
		t.Fatal("expected wasCancelled to be true after requestCancel")
	}

	sess.startTurn(func() {})
	if sess.wasCancelled() {
		t.Error("startTurn must clear a cancelled flag left over from before this turn")
	}
}

func TestSession_RequestCancel_CallsCurrentTurnsCancelFunc(t *testing.T) {
	sess := newSession("s1")
	called := false
	sess.startTurn(func() { called = true })

	sess.requestCancel()

	if !called {
		t.Error("requestCancel must call the in-flight turn's cancel func")
	}
	if !sess.wasCancelled() {
		t.Error("requestCancel must mark the session cancelled")
	}
}

func TestSession_RequestCancel_NoInFlightTurn_DoesNotPanic(t *testing.T) {
	sess := newSession("s1")
	sess.requestCancel() // no startTurn called yet — cancel func is nil
	if !sess.wasCancelled() {
		t.Error("requestCancel must still mark the session cancelled with no turn in flight")
	}
}

func TestSession_EndTurn_DisarmsCancelFunc(t *testing.T) {
	sess := newSession("s1")
	called := false
	sess.startTurn(func() { called = true })
	sess.endTurn()

	sess.requestCancel()

	if called {
		t.Error("requestCancel must not call a cancel func from a turn that already ended")
	}
	if !sess.wasCancelled() {
		t.Error("requestCancel must still record the cancel attempt even with no turn in flight")
	}
}

func TestSession_StartTurn_Twice_OnlyArmsLatestCancelFunc(t *testing.T) {
	sess := newSession("s1")
	var firstCalled, secondCalled bool
	sess.startTurn(func() { firstCalled = true })
	sess.endTurn()
	sess.startTurn(func() { secondCalled = true })

	sess.requestCancel()

	if firstCalled {
		t.Error("requestCancel must not call a previous turn's cancel func")
	}
	if !secondCalled {
		t.Error("requestCancel must call the current turn's cancel func")
	}
}

func TestSession_StartTurn_RejectsWhenAlreadyInFlight(t *testing.T) {
	sess := newSession("s1")
	if !sess.startTurn(func() {}) {
		t.Fatal("first startTurn on an idle session should succeed")
	}
	if sess.startTurn(func() {}) {
		t.Error("startTurn should reject a second call while a turn is already in flight, not silently overwrite it")
	}
}

func TestSession_StartTurn_SucceedsAgainAfterEndTurn(t *testing.T) {
	sess := newSession("s1")
	sess.startTurn(func() {})
	sess.endTurn()
	if !sess.startTurn(func() {}) {
		t.Error("startTurn should succeed again once the prior turn has ended")
	}
}

func TestSession_ComposeQuery_NoHistoryReturnsQueryUnchanged(t *testing.T) {
	sess := newSession("s1")
	got := sess.composeQuery("hello")
	if got != "hello" {
		t.Errorf("got %q, want the query unchanged when there's no recorded history", got)
	}
}

func TestSession_ComposeQuery_IncludesPriorTurns(t *testing.T) {
	sess := newSession("s1")
	sess.recordTurn("first question", "first answer")
	got := sess.composeQuery("second question")
	if !strings.Contains(got, "first question") {
		t.Errorf("composed query missing the prior question: %q", got)
	}
	if !strings.Contains(got, "first answer") {
		t.Errorf("composed query missing the prior answer: %q", got)
	}
	if !strings.Contains(got, "second question") {
		t.Errorf("composed query missing the new question: %q", got)
	}
}

func TestSession_ComposeQuery_TrimsToMaxTurns(t *testing.T) {
	sess := newSession("s1")
	for i := 0; i < historyMaxTurns+3; i++ {
		sess.recordTurn(fmt.Sprintf("q%d", i), fmt.Sprintf("a%d", i))
	}
	got := sess.composeQuery("latest")
	if strings.Contains(got, "q0") {
		t.Error("oldest turn should have been trimmed out of the composed history")
	}
	if !strings.Contains(got, fmt.Sprintf("q%d", historyMaxTurns+2)) {
		t.Error("most recent prior turn should still be present in the composed history")
	}
}

func TestSession_RecordTurn_BoundsStorageNotJustComposeOutput(t *testing.T) {
	sess := newSession("s1")
	for i := 0; i < historyMaxTurns+3; i++ {
		sess.recordTurn(fmt.Sprintf("q%d", i), fmt.Sprintf("a%d", i))
	}

	sess.mu.Lock()
	got := len(sess.history)
	sess.mu.Unlock()

	if got != historyMaxTurns {
		t.Errorf("stored history has %d turns, want it trimmed to historyMaxTurns (%d) at record time, not just at compose time", got, historyMaxTurns)
	}
}

func TestSession_RecordTurn_TruncatesMultiByteResponseWithoutSplittingRunes(t *testing.T) {
	sess := newSession("s1")
	// Japanese text: 3 bytes/rune, well over historyMaxResponseChars runes,
	// so a byte-index truncation is virtually certain to land mid-rune.
	longResponse := strings.Repeat("こんにちは", historyMaxResponseChars) // "こんにちは" x N
	sess.recordTurn("question", longResponse)

	got := sess.composeQuery("next question")

	if !utf8.ValidString(got) {
		t.Fatal("composed query is not valid UTF-8 — response was truncated mid-rune")
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Error("composed query contains U+FFFD, meaning the stored response was corrupted by a byte-index cut through a multi-byte rune")
	}
}

func TestSession_RecordTurn_TruncatesLongQueryTooNotJustResponse(t *testing.T) {
	sess := newSession("s1")
	// A pasted diff or long prompt is exactly the shape server.go's own
	// reader-loop comment anticipates — query deserves the same bound as
	// response, since it's replayed into every one of the next
	// historyMaxTurns prompts otherwise.
	longQuery := strings.Repeat("q", historyMaxResponseChars+500)
	sess.recordTurn(longQuery, "short answer")

	sess.mu.Lock()
	stored := sess.history[0].query
	sess.mu.Unlock()

	if len([]rune(stored)) > historyMaxResponseChars+len("...[truncated]") {
		t.Errorf("stored query has %d runes, want it truncated to around historyMaxResponseChars (%d) like response already is", len([]rune(stored)), historyMaxResponseChars)
	}
}

func TestSessionMap_AddGet(t *testing.T) {
	m := newSessionMap()
	sess := newSession("s1")
	m.add(sess)

	got, ok := m.get("s1")
	if !ok {
		t.Fatal("expected to find session s1")
	}
	if got != sess {
		t.Error("get returned a different session than the one added")
	}
}

func TestSessionMap_Get_NotFound(t *testing.T) {
	m := newSessionMap()
	_, ok := m.get("nonexistent")
	if ok {
		t.Error("expected ok=false for an unknown session id")
	}
}
