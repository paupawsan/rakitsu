package acp

import "testing"

// Regression: agent/cancel sets status to "cancelled" via sess.cancelled(),
// but the run goroutine's own error path calls sess.fail() shortly after
// once it observes the cancelled context — without a sticky terminal state,
// that overwrites "cancelled" back to "error" and a client polling
// agent/status sees the wrong terminal state.

func TestSession_CancelThenFail_StaysCancelled(t *testing.T) {
	sess := newSession("s1", func() {})
	sess.cancelled()
	sess.fail("context canceled")

	status, _, errMsg := sess.snapshot()
	if status != statusCancelled {
		t.Errorf("status = %q, want %q — fail() after cancelled() must be a no-op", status, statusCancelled)
	}
	if errMsg != "" {
		t.Errorf("errMsg = %q, want empty — fail() after cancelled() must not record a message", errMsg)
	}
}

func TestSession_CancelThenComplete_StaysCancelled(t *testing.T) {
	sess := newSession("s1", func() {})
	sess.cancelled()
	sess.complete("some result")

	status, result, _ := sess.snapshot()
	if status != statusCancelled {
		t.Errorf("status = %q, want %q — complete() after cancelled() must be a no-op", status, statusCancelled)
	}
	if result != "" {
		t.Errorf("result = %q, want empty", result)
	}
}

// Regression, opposite ordering: complete()/fail() lands first, then a
// late agent/cancel arrives before the session is removed from the
// registry (handleRun's deferred delete only runs after the goroutine
// already reported "agent/complete"). Without a guard on cancelled() too,
// that overwrites the real outcome back to "cancelled" while result/errMsg
// still hold the completed values — an internally inconsistent snapshot.

func TestSession_CompleteThenCancelled_StaysCompleted(t *testing.T) {
	sess := newSession("s1", func() {})
	sess.complete("real result")
	sess.cancelled()

	status, result, _ := sess.snapshot()
	if status != statusCompleted {
		t.Errorf("status = %q, want %q — cancelled() after complete() must be a no-op", status, statusCompleted)
	}
	if result != "real result" {
		t.Errorf("result = %q, want %q", result, "real result")
	}
}

func TestSession_FailThenCancelled_StaysError(t *testing.T) {
	sess := newSession("s1", func() {})
	sess.fail("boom")
	sess.cancelled()

	status, _, errMsg := sess.snapshot()
	if status != statusError {
		t.Errorf("status = %q, want %q — cancelled() after fail() must be a no-op", status, statusError)
	}
	if errMsg != "boom" {
		t.Errorf("errMsg = %q, want %q", errMsg, "boom")
	}
}

// Sanity check the normal (non-cancelled) paths are unaffected by the guard.

func TestSession_Fail_RecordsError(t *testing.T) {
	sess := newSession("s1", func() {})
	sess.fail("boom")

	status, _, errMsg := sess.snapshot()
	if status != statusError || errMsg != "boom" {
		t.Errorf("snapshot = (%q, %q), want (%q, %q)", status, errMsg, statusError, "boom")
	}
}

func TestSession_Complete_RecordsResult(t *testing.T) {
	sess := newSession("s1", func() {})
	sess.complete("done")

	status, result, _ := sess.snapshot()
	if status != statusCompleted || result != "done" {
		t.Errorf("snapshot = (%q, %q), want (%q, %q)", status, result, statusCompleted, "done")
	}
}

// TestSession_CompleteThenFail_KeepsCompleted and its mirror below guard a
// symmetry gap: complete() and fail() each now also refuse to overwrite the
// OTHER terminal state, not just cancelled — matching cancelled()'s own
// guard above, which already checked both.

func TestSession_CompleteThenFail_KeepsCompleted(t *testing.T) {
	s := newSession("s1", func() {})
	s.complete("ok")
	s.fail("should not override")

	status, result, errMsg := s.snapshot()
	if status != statusCompleted {
		t.Errorf("want status %q, got %q", statusCompleted, status)
	}
	if result != "ok" {
		t.Errorf("want result %q, got %q", "ok", result)
	}
	if errMsg != "" {
		t.Errorf("want no error message, got %q", errMsg)
	}
}

func TestSession_FailThenComplete_KeepsError(t *testing.T) {
	s := newSession("s1", func() {})
	s.fail("boom")
	s.complete("should not override")

	status, result, errMsg := s.snapshot()
	if status != statusError {
		t.Errorf("want status %q, got %q", statusError, status)
	}
	if errMsg != "boom" {
		t.Errorf("want error message %q, got %q", "boom", errMsg)
	}
	if result != "" {
		t.Errorf("want no result, got %q", result)
	}
}
