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
