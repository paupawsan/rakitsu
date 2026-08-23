package store

import (
	"testing"
	"time"
)

func TestHasCheckpoint_FalseWhenMissing(t *testing.T) {
	s := newTestStore(t)
	startTestSession(t, s)
	id := s.CurrentSessionID()

	if s.HasCheckpoint(id) {
		t.Error("HasCheckpoint should be false before any checkpoint is written")
	}
}

func TestHasCheckpoint_TrueAfterWrite(t *testing.T) {
	s := newTestStore(t)
	startTestSession(t, s)
	id := s.CurrentSessionID()

	err := s.WriteCheckpoint(StoreCheckpointData{
		SessionID:      id,
		Query:          "test",
		CompletedSteps: []string{"step1"},
		Results:        map[string]StoreStepResult{"step1": {Name: "step1", Output: "done"}},
	})
	if err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	if !s.HasCheckpoint(id) {
		t.Error("HasCheckpoint should be true after WriteCheckpoint")
	}
}

func TestHasCheckpoint_InvalidIDReturnsFalse(t *testing.T) {
	s := newTestStore(t)
	// IDs with path traversal characters must return false, not panic
	if s.HasCheckpoint("../../etc/passwd") {
		t.Error("HasCheckpoint should reject path-traversal IDs")
	}
}

func TestListSessions_Empty(t *testing.T) {
	s := newTestStore(t)
	sessions, err := s.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions on empty store: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

// TestGetSession_ReclassifiesOrphanedRunningAsStale regression-guards
// against GetSession and ListSessions disagreeing about the same session's
// status: ListSessions already reclassifies an orphaned "running" session
// (any ID other than the current active one — e.g. left behind by a crashed
// process) as "stale" before returning it, but GetSession skipped that same
// logic and returned the raw indexed status forever.
func TestGetSession_ReclassifiesOrphanedRunningAsStale(t *testing.T) {
	s := newTestStore(t)

	// Simulate a crashed run: StartSession without a matching EndSession
	// leaves the index entry at Status=running, and s.current pointing at
	// it. A fresh store instance loaded later (the real-world crash-recovery
	// path) has s.current == nil, so this session is "orphaned running" as
	// far as any store instance reading the index is concerned.
	if err := s.StartSession(SessionMeta{Name: "crashed", Query: "q"}); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	id := s.CurrentSessionID()

	// Reload against the same directory with no in-memory current session,
	// as rakitsu would after a restart.
	reloaded := &SessionStore{dir: s.dir}

	got, err := reloaded.GetSession(id)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Status != SessionStale {
		t.Errorf("GetSession Status = %q, want %q (orphaned running session)", got.Status, SessionStale)
	}

	// ListSessions must agree.
	list, err := reloaded.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list) != 1 || list[0].Status != SessionStale {
		t.Errorf("ListSessions = %+v, want single entry with Status=%q", list, SessionStale)
	}
}

// TestGetSession_CurrentSessionStaysRunning verifies the reclassification
// doesn't misfire on the store's own active session — only sessions OTHER
// than s.current are stale candidates.
func TestGetSession_CurrentSessionStaysRunning(t *testing.T) {
	s := newTestStore(t)
	id := startTestSession(t, s)

	got, err := s.GetSession(id)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Status != SessionRunning {
		t.Errorf("GetSession Status = %q, want %q (still the active session)", got.Status, SessionRunning)
	}
}

func TestListSessions_OrderedNewestFirst(t *testing.T) {
	s := newTestStore(t)

	// Create 3 sessions with slight time separation
	names := []string{"first", "second", "third"}
	for _, name := range names {
		err := s.StartSession(SessionMeta{Name: name, Query: name + " query"})
		if err != nil {
			t.Fatalf("StartSession(%s): %v", name, err)
		}
		s.EndSession(SessionSuccess)
		time.Sleep(2 * time.Millisecond) // ensure distinct timestamps
	}

	sessions, err := s.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
	// Newest first
	if sessions[0].Name != "third" {
		t.Errorf("sessions[0].Name = %q, want %q", sessions[0].Name, "third")
	}
	if sessions[2].Name != "first" {
		t.Errorf("sessions[2].Name = %q, want %q", sessions[2].Name, "first")
	}
}
