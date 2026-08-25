package server

import (
	"testing"
	"time"
)

// TestSubmit_DoesNotBlockForeverWhenClosedWithFullQueue regression-guards:
// Submit's `s.turnCh <- chatTurn{...}` is an unconditional blocking send
// with no select against s.stopCh. runLoop's own select does race stopCh
// against turnCh, but if Close() runs while turnCh's 1-slot buffer is
// already full (a second concurrent Submit racing an in-flight turn) and
// runLoop's select happens to pick the now-closed stopCh case instead of
// draining the buffered turn, runLoop returns and nothing will ever receive
// from turnCh again — the blocked Submit caller's goroutine is stuck
// permanently. Reproduced deterministically here by filling turnCh's
// buffer directly (no runLoop running to drain it) and closing stopCh
// before Submit's send — proves the send itself respects stopCh, which is
// what actually closes the leak regardless of runLoop's own select timing.
func TestSubmit_DoesNotBlockForeverWhenClosedWithFullQueue(t *testing.T) {
	sess := &ChatSession{
		turnCh: make(chan chatTurn, 1),
		stopCh: make(chan struct{}),
	}
	sess.turnCh <- chatTurn{text: "already queued"} // fill the one slot
	close(sess.stopCh)                              // simulate Close() racing in

	done := make(chan error, 1)
	go func() { done <- sess.Submit("new turn") }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("Submit on a stopped session with a full queue returned nil, want an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Submit blocked forever instead of returning once the session was stopped")
	}
}

// TestSubmitOnActiveLeaf_DoesNotBlockForeverWhenClosedWithFullQueue is the
// same regression, same fix, for SubmitOnActiveLeaf's equivalent send.
func TestSubmitOnActiveLeaf_DoesNotBlockForeverWhenClosedWithFullQueue(t *testing.T) {
	sess := &ChatSession{
		turnCh: make(chan chatTurn, 1),
		stopCh: make(chan struct{}),
	}
	sess.turnCh <- chatTurn{text: "already queued"}
	close(sess.stopCh)

	done := make(chan error, 1)
	go func() { done <- sess.SubmitOnActiveLeaf("new turn") }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("SubmitOnActiveLeaf on a stopped session with a full queue returned nil, want an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SubmitOnActiveLeaf blocked forever instead of returning once the session was stopped")
	}
}
