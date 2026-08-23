package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/paupawsan/rakitsu/internal/telemetry"
)

// newTestStore creates a SessionStore backed by a temp directory.
func newTestStore(t *testing.T) *SessionStore {
	t.Helper()
	dir := t.TempDir()
	return &SessionStore{dir: dir}
}

// startTestSession starts a session and returns the session ID.
func startTestSession(t *testing.T, s *SessionStore) string {
	t.Helper()
	err := s.StartSession(SessionMeta{
		Name:  "test",
		Query: "test query",
	})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	return s.CurrentSessionID()
}

// --- WriteCheckpoint / LoadCheckpoint ---

func TestCheckpoint_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	startTestSession(t, s)

	data := StoreCheckpointData{
		SessionID:      s.CurrentSessionID(),
		Query:          "build something",
		CompletedSteps: []string{"plan", "implement"},
		Results: map[string]StoreStepResult{
			"plan":      {Name: "plan", Output: "plan output", DurationNs: 1_000_000_000},
			"implement": {Name: "implement", Output: "code output", DurationNs: 5_000_000_000},
		},
	}

	if err := s.WriteCheckpoint(data); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	loaded, err := s.LoadCheckpoint(s.CurrentSessionID())
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	if loaded.Query != data.Query {
		t.Errorf("Query = %q, want %q", loaded.Query, data.Query)
	}
	if len(loaded.CompletedSteps) != 2 {
		t.Errorf("CompletedSteps len = %d, want 2", len(loaded.CompletedSteps))
	}
	if loaded.Results["plan"].Output != "plan output" {
		t.Errorf("plan output = %q, want %q", loaded.Results["plan"].Output, "plan output")
	}
	if loaded.Results["implement"].DurationNs != 5_000_000_000 {
		t.Errorf("implement DurationNs = %d, want 5e9", loaded.Results["implement"].DurationNs)
	}
}

func TestCheckpoint_OverwriteIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	startTestSession(t, s)

	write := func(steps []string) {
		_ = s.WriteCheckpoint(StoreCheckpointData{
			SessionID:      s.CurrentSessionID(),
			CompletedSteps: steps,
			Results:        map[string]StoreStepResult{},
		})
	}

	write([]string{"plan"})
	write([]string{"plan", "implement"})
	write([]string{"plan", "implement", "verify"})

	loaded, err := s.LoadCheckpoint(s.CurrentSessionID())
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if len(loaded.CompletedSteps) != 3 {
		t.Errorf("CompletedSteps = %v, want 3 steps", loaded.CompletedSteps)
	}
}

func TestCheckpoint_LoadMissingReturnsError(t *testing.T) {
	s := newTestStore(t)
	_, err := s.LoadCheckpoint("nonexistent-session-id")
	if err == nil {
		t.Fatal("LoadCheckpoint on missing session should return error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error should wrap os.ErrNotExist, got: %v", err)
	}
}

func TestCheckpoint_InvalidSessionIDRejected(t *testing.T) {
	s := newTestStore(t)
	_, err := s.LoadCheckpoint("../../etc/passwd")
	if err == nil {
		t.Fatal("LoadCheckpoint with path traversal ID should return error")
	}
}

// TestCheckpoint_WriteIsAtomicNoTmpLeak regression-guards finding 20:
// WriteCheckpoint used to write the checkpoint file directly via
// os.WriteFile (truncate-then-write in place), unlike SaveChatTree's
// tmp+rename pattern in the same package — a concurrent LoadCheckpoint
// (which reads with no lock of its own, since a checkpoint can be loaded
// for any session id, not just the current one) could observe a
// half-written file. After a successful write there must be no .tmp file
// left behind.
func TestCheckpoint_WriteIsAtomicNoTmpLeak(t *testing.T) {
	s := newTestStore(t)
	startTestSession(t, s)

	if err := s.WriteCheckpoint(StoreCheckpointData{
		SessionID:      s.CurrentSessionID(),
		CompletedSteps: []string{"plan"},
		Results:        map[string]StoreStepResult{},
	}); err != nil {
		t.Fatalf("WriteCheckpoint: %v", err)
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("stray tmp file after WriteCheckpoint: %s", e.Name())
		}
	}
}

func TestCheckpoint_WriteRequiresActiveSession(t *testing.T) {
	s := newTestStore(t)
	// No session started
	err := s.WriteCheckpoint(StoreCheckpointData{})
	if err == nil {
		t.Fatal("WriteCheckpoint without active session should return error")
	}
}

// --- DeleteSession removes checkpoint file ---

func TestDeleteSession_RemovesCheckpointFile(t *testing.T) {
	s := newTestStore(t)
	id := startTestSession(t, s)

	// Write a checkpoint
	_ = s.WriteCheckpoint(StoreCheckpointData{
		SessionID:      id,
		CompletedSteps: []string{"plan"},
		Results:        map[string]StoreStepResult{},
	})

	// End session then delete
	s.EndSession(SessionSuccess)
	if err := s.DeleteSession(id); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	// Checkpoint file should be gone
	_, err := s.LoadCheckpoint(id)
	if err == nil {
		t.Error("checkpoint file should be removed after DeleteSession")
	}
}

func TestDeleteSession_NoCheckpointIsNoop(t *testing.T) {
	s := newTestStore(t)
	id := startTestSession(t, s)
	s.EndSession(SessionSuccess)

	// No checkpoint written — delete should still succeed
	if err := s.DeleteSession(id); err != nil {
		t.Errorf("DeleteSession without checkpoint should not error: %v", err)
	}
}

// TestDeleteSession_RemovesChatTree regression-guards against a .chat.json
// blob being left orphaned on disk forever: DeleteSession already removed
// the .jsonl and .checkpoint.json files for a session but not its chat tree
// blob, which is keyed by the same session id and tracked nowhere else.
func TestDeleteSession_RemovesChatTree(t *testing.T) {
	s := newTestStore(t)
	id := startTestSession(t, s)

	if err := s.SaveChatTree(id, []byte(`{"tree":{}}`)); err != nil {
		t.Fatalf("SaveChatTree: %v", err)
	}
	s.EndSession(SessionSuccess)

	if err := s.DeleteSession(id); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if _, err := s.LoadChatTree(id); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("chat tree should be removed after DeleteSession, LoadChatTree err = %v", err)
	}
}

// TestDeleteSession_ConcurrentWithStartSessionIndexRace regression-guards
// finding 19: DeleteSession used to read-modify-write sessions.json without
// holding s.mu, so on one shared *SessionStore instance (the shape
// `rakitsu serve`'s hub actually uses — one store, API delete handlers and
// the runner's StartSession/EndSession calls all running as goroutines
// against it) DeleteSession could race a concurrent StartSession/EndSession
// (which do hold s.mu around their own index read-modify-write) and lose
// one side's update. Deletes half of a batch of sessions concurrently with
// starting+ending a fresh batch on the SAME store instance, and asserts the
// index ends up with exactly the sessions that should have survived — a
// lost update would under- or over-count. The StartSession/EndSession pairs
// are serialized against each other via startMu (a single active session is
// a store-wide invariant unrelated to this finding); only DeleteSession is
// left free to interleave with them, which is what this test targets.
func TestDeleteSession_ConcurrentWithStartSessionIndexRace(t *testing.T) {
	s := newTestStore(t)

	const n = 20
	ids := make([]string, n)
	for i := range ids {
		if err := s.StartSession(SessionMeta{Name: fmt.Sprintf("pre-%d", i), Query: "q"}); err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		ids[i] = s.CurrentSessionID()
		s.EndSession(SessionSuccess)
	}

	var wg sync.WaitGroup
	var startMu sync.Mutex // serializes StartSession/EndSession pairs against each other only

	// Delete the first half concurrently with starting+ending a second
	// batch of brand-new sessions, all against the same store instance.
	for i := 0; i < n/2; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := s.DeleteSession(id); err != nil {
				t.Errorf("DeleteSession(%s): %v", id, err)
			}
		}(ids[i])
	}
	for i := 0; i < n/2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			startMu.Lock()
			defer startMu.Unlock()
			if err := s.StartSession(SessionMeta{Name: fmt.Sprintf("post-%d", i), Query: "q"}); err != nil {
				t.Errorf("StartSession: %v", err)
				return
			}
			s.EndSession(SessionSuccess)
		}(i)
	}
	wg.Wait()

	list, err := s.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	// n/2 pre-sessions survived deletion of the other half, plus n/2 new
	// post-sessions were added: n total either way.
	if len(list) != n {
		t.Errorf("ListSessions returned %d entries, want %d (a lost update during concurrent index writes would under/over-count)", len(list), n)
	}
}

// --- SESSION_END as last JSONL line ---

// TestEndSession_WritesSessionEndAsLastLine verifies that EndSession writes a
// SESSION_END event row as the final JSONL line, with payload mirroring the
// final SessionMeta status/totals — so consumers reading the stream alone
// (without sessions.json) can determine final status from the last line.
// Regression guard.
func TestEndSession_WritesSessionEndAsLastLine(t *testing.T) {
	s := newTestStore(t)
	id := startTestSession(t, s)

	s.WriteEvent(telemetry.AgentEvent{
		ID:        "ev-1",
		EventType: telemetry.EventAgentStart,
		AgentName: "alpha",
	})
	s.WriteEvent(telemetry.AgentEvent{
		ID:        "ev-2",
		EventType: telemetry.EventAgentEnd,
		AgentName: "alpha",
		TokenUsage: &telemetry.TokenUsage{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	})

	s.EndSession(SessionSuccess)

	data, err := os.ReadFile(filepath.Join(s.dir, id+".jsonl"))
	if err != nil {
		t.Fatalf("read session file: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	// Expected: line 0 = SessionMeta, line 1 = AGENT_START, line 2 = AGENT_END,
	// line 3 = SESSION_END.
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (meta + 2 events + SESSION_END); got %d:\n%s",
			len(lines), strings.Join(lines, "\n"))
	}

	var endEvent telemetry.AgentEvent
	if err := json.Unmarshal([]byte(lines[3]), &endEvent); err != nil {
		t.Fatalf("unmarshal last line: %v\nline: %s", err, lines[3])
	}
	if endEvent.EventType != telemetry.EventSessionEnd {
		t.Errorf("last event type = %q, want %q", endEvent.EventType, telemetry.EventSessionEnd)
	}
	if endEvent.SessionID != id {
		t.Errorf("SESSION_END session_id = %q, want %q", endEvent.SessionID, id)
	}

	var payload telemetry.SessionEndPayload
	if err := json.Unmarshal(endEvent.Payload, &payload); err != nil {
		t.Fatalf("unmarshal SESSION_END payload: %v", err)
	}
	if payload.Status != telemetry.SessionEndSuccess {
		t.Errorf("status = %q, want %q", payload.Status, telemetry.SessionEndSuccess)
	}
	if payload.TotalEvents != 2 {
		t.Errorf("total_events = %d, want 2 (SESSION_END itself excluded)", payload.TotalEvents)
	}
	if payload.TotalTokens != 30 {
		t.Errorf("total_tokens = %d, want 30", payload.TotalTokens)
	}
	if payload.DurationMs < 0 {
		t.Errorf("duration_ms = %d, want non-negative", payload.DurationMs)
	}
}

// TestEndSession_PropagatesErrorStatus verifies that EndSession with an error
// status writes the same status into the SESSION_END event payload, so a
// consumer reading only the JSONL can distinguish success/error/timeout/stale
// without consulting the index file.
func TestEndSession_PropagatesErrorStatus(t *testing.T) {
	cases := []SessionStatus{SessionError, SessionTimeout, SessionStale}
	for _, status := range cases {
		t.Run(string(status), func(t *testing.T) {
			s := newTestStore(t)
			id := startTestSession(t, s)
			s.EndSession(status)

			data, err := os.ReadFile(filepath.Join(s.dir, id+".jsonl"))
			if err != nil {
				t.Fatalf("read session file: %v", err)
			}
			lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
			lastLine := lines[len(lines)-1]

			var endEvent telemetry.AgentEvent
			if err := json.Unmarshal([]byte(lastLine), &endEvent); err != nil {
				t.Fatalf("unmarshal last line: %v", err)
			}
			var payload telemetry.SessionEndPayload
			if err := json.Unmarshal(endEvent.Payload, &payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			if string(payload.Status) != string(status) {
				t.Errorf("payload.status = %q, want %q", payload.Status, status)
			}
		})
	}
}

// TestGetSessionEvents_ExcludesSessionEnd verifies that the SESSION_END
// marker written by EndSession is filtered out of GetSessionEvents results,
// so consumers comparing len(events) to SessionEndPayload.TotalEvents see
// matching values. Regression guard for the M2 review concern.
func TestGetSessionEvents_ExcludesSessionEnd(t *testing.T) {
	s := newTestStore(t)
	id := startTestSession(t, s)

	const wantCount = 3
	for i := 0; i < wantCount; i++ {
		s.WriteEvent(telemetry.AgentEvent{
			ID:        fmt.Sprintf("ev-%d", i),
			EventType: telemetry.EventAgentStart,
			AgentName: "alpha",
		})
	}
	s.EndSession(SessionSuccess)

	events, err := s.GetSessionEvents(id)
	if err != nil {
		t.Fatalf("GetSessionEvents: %v", err)
	}
	if len(events) != wantCount {
		t.Errorf("len(events) = %d, want %d (SESSION_END must be filtered)", len(events), wantCount)
	}
	for _, ev := range events {
		if ev.EventType == telemetry.EventSessionEnd {
			t.Errorf("GetSessionEvents leaked SESSION_END marker (id=%s)", ev.ID)
		}
	}
}

// --- Stress tests ---

// TestCheckpoint_ConcurrentWrites verifies that concurrent WriteCheckpoint calls
// are safe (atomic os.WriteFile overwrite, protected by store mutex).
func TestCheckpoint_ConcurrentWrites(t *testing.T) {
	s := newTestStore(t)
	startTestSession(t, s)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			_ = s.WriteCheckpoint(StoreCheckpointData{
				SessionID:      s.CurrentSessionID(),
				CompletedSteps: []string{"plan"},
				Results: map[string]StoreStepResult{
					"plan": {Name: "plan", Output: "output", DurationNs: int64(i * 1_000_000)},
				},
			})
		}(i)
	}
	wg.Wait()

	// File should be readable and valid JSON after concurrent writes
	loaded, err := s.LoadCheckpoint(s.CurrentSessionID())
	if err != nil {
		t.Fatalf("LoadCheckpoint after concurrent writes: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded checkpoint is nil")
	}
}

// TestCheckpoint_LargeOutput verifies checkpoints handle large step outputs correctly.
func TestCheckpoint_LargeOutput(t *testing.T) {
	s := newTestStore(t)
	startTestSession(t, s)

	// 1MB output per step
	largeOutput := make([]byte, 1<<20)
	for i := range largeOutput {
		largeOutput[i] = byte('a' + (i % 26))
	}

	data := StoreCheckpointData{
		SessionID:      s.CurrentSessionID(),
		CompletedSteps: []string{"plan"},
		Results: map[string]StoreStepResult{
			"plan": {Name: "plan", Output: string(largeOutput), DurationNs: 1_000_000},
		},
	}

	if err := s.WriteCheckpoint(data); err != nil {
		t.Fatalf("WriteCheckpoint large output: %v", err)
	}

	loaded, err := s.LoadCheckpoint(s.CurrentSessionID())
	if err != nil {
		t.Fatalf("LoadCheckpoint large output: %v", err)
	}
	if len(loaded.Results["plan"].Output) != 1<<20 {
		t.Errorf("large output length = %d, want %d", len(loaded.Results["plan"].Output), 1<<20)
	}
}

// TestCheckpoint_Stress100Steps simulates 100 sequential checkpoint writes and
// verifies final state is consistent.
func TestCheckpoint_Stress100Steps(t *testing.T) {
	s := newTestStore(t)
	startTestSession(t, s)

	const totalSteps = 100
	completed := make([]string, 0, totalSteps)
	results := make(map[string]StoreStepResult, totalSteps)

	for i := 0; i < totalSteps; i++ {
		name := fmt.Sprintf("step-%03d", i)
		completed = append(completed, name)
		results[name] = StoreStepResult{
			Name:       name,
			Output:     fmt.Sprintf("output for %s", name),
			DurationNs: int64(i) * 1_000_000,
		}

		if err := s.WriteCheckpoint(StoreCheckpointData{
			SessionID:      s.CurrentSessionID(),
			CompletedSteps: append([]string(nil), completed...),
			Results:        copyResults(results),
		}); err != nil {
			t.Fatalf("WriteCheckpoint at step %d: %v", i, err)
		}
	}

	loaded, err := s.LoadCheckpoint(s.CurrentSessionID())
	if err != nil {
		t.Fatalf("LoadCheckpoint after 100 steps: %v", err)
	}
	if len(loaded.CompletedSteps) != totalSteps {
		t.Errorf("CompletedSteps len = %d, want %d", len(loaded.CompletedSteps), totalSteps)
	}
	if len(loaded.Results) != totalSteps {
		t.Errorf("Results len = %d, want %d", len(loaded.Results), totalSteps)
	}
}

func copyResults(m map[string]StoreStepResult) map[string]StoreStepResult {
	out := make(map[string]StoreStepResult, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// suppress unused import if fmt isn't used elsewhere
var _ = time.Second
