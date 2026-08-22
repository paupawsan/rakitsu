package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustAdd(t *testing.T, s *Store, n Node) AddResult {
	t.Helper()
	res, err := s.Add(n)
	if err != nil {
		t.Fatalf("Add %s: %v", n.ID, err)
	}
	return res
}

func TestRetireHidesFromQueryAndList(t *testing.T) {
	s := newTestStore(t)
	mustAdd(t, s, Node{ID: "old-way", Type: "fact", Title: "deploy checklist", Content: "deploy checklist", Scope: ScopeGlobal})
	if err := s.Retire(ScopeGlobal, "old-way"); err != nil {
		t.Fatalf("Retire: %v", err)
	}
	res, err := s.Query("deploy checklist", []string{ScopeGlobal}, "", 10, View{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("retired node must not match current-view query, got %d results", len(res))
	}
	out, err := s.List(ScopeGlobal, "", 0, View{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("retired node must not appear in current-view list, got %d", len(out))
	}
}

func TestRetireMissingAndAlreadyRetiredFail(t *testing.T) {
	s := newTestStore(t)
	if err := s.Retire(ScopeGlobal, "nope"); err == nil {
		t.Error("expected error retiring a missing node")
	}
	mustAdd(t, s, Node{ID: "x", Type: "note", Title: "x", Scope: ScopeGlobal})
	if err := s.Retire(ScopeGlobal, "x"); err != nil {
		t.Fatalf("Retire: %v", err)
	}
	if err := s.Retire(ScopeGlobal, "x"); err == nil || !strings.Contains(err.Error(), "already retired") {
		t.Errorf("expected already-retired error, got %v", err)
	}
}

func TestAddSupersedingInvalidatesOldAndLinks(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	mustAdd(t, s, Node{ID: "old", Type: "fact", Title: "deploy checklist", Content: "deploy checklist v1", Scope: ScopeGlobal})
	if _, err := s.AddSuperseding(Node{ID: "new", Type: "fact", Title: "deploy checklist", Content: "deploy checklist v2", Scope: ScopeGlobal}, "old"); err != nil {
		t.Fatalf("AddSuperseding: %v", err)
	}

	old, _ := s.Get(ScopeGlobal, "old")
	if old.InvalidAt == nil {
		t.Fatal("superseded node must have InvalidAt set")
	}
	if old.ExpiredAt != nil {
		t.Error("supersede must not set ExpiredAt (that is retirement)")
	}
	res, err := s.Query("deploy checklist", []string{ScopeGlobal}, "", 10, View{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) != 1 || res[0].ID != "new" {
		t.Errorf("current view must return only the replacement, got %+v", res)
	}
	if len(res[0].Links) != 1 || res[0].Links[0].Target != "old" || res[0].Links[0].Relation != "supersedes" {
		t.Errorf("new node must carry a supersedes link to old, got %+v", res[0].Links)
	}

	// Both writes landed in one atomic save: a fresh Store sees them.
	s2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore reopen: %v", err)
	}
	old2, err := s2.Get(ScopeGlobal, "old")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if old2.InvalidAt == nil {
		t.Error("InvalidAt must survive a reload")
	}
}

func TestAddSupersedingMissingTargetFails(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddSuperseding(Node{ID: "new", Type: "fact", Title: "n", Scope: ScopeGlobal}, "ghost"); err == nil {
		t.Fatal("expected error for missing supersede target")
	}
	if _, err := s.Get(ScopeGlobal, "new"); err == nil {
		t.Error("failed supersede must not store the new node")
	}
}

func TestAddSupersedingSelfFails(t *testing.T) {
	s := newTestStore(t)
	mustAdd(t, s, Node{ID: "a", Type: "fact", Title: "a", Scope: ScopeGlobal})
	if _, err := s.AddSuperseding(Node{ID: "a", Type: "fact", Title: "a", Scope: ScopeGlobal}, "a"); err == nil {
		t.Error("expected error superseding self")
	}
}

func TestAddSupersedingKeepsEarliestInvalidAt(t *testing.T) {
	s := newTestStore(t)
	mustAdd(t, s, Node{ID: "old", Type: "fact", Title: "old", Scope: ScopeGlobal})
	if _, err := s.AddSuperseding(Node{ID: "n1", Type: "fact", Title: "n1", Scope: ScopeGlobal}, "old"); err != nil {
		t.Fatalf("first supersede: %v", err)
	}
	first, _ := s.Get(ScopeGlobal, "old")
	time.Sleep(5 * time.Millisecond)
	if _, err := s.AddSuperseding(Node{ID: "n2", Type: "fact", Title: "n2", Scope: ScopeGlobal}, "old"); err != nil {
		t.Fatalf("second supersede: %v", err)
	}
	again, _ := s.Get(ScopeGlobal, "old")
	if !again.InvalidAt.Equal(*first.InvalidAt) {
		t.Errorf("re-superseding must keep the earliest InvalidAt: %v vs %v", again.InvalidAt, first.InvalidAt)
	}
}

// TestAddSupersedingDoesNotBumpUpdatedOnResupersede pins that a repeat
// supersede of an already-invalidated node is a no-op on Updated: nothing
// about "old" actually changed (InvalidAt keeps its earliest stamp), so its
// bm25 recency-boost signal must not be refreshed by an unrelated later event.
func TestAddSupersedingDoesNotBumpUpdatedOnResupersede(t *testing.T) {
	s := newTestStore(t)
	mustAdd(t, s, Node{ID: "old", Type: "fact", Title: "old", Scope: ScopeGlobal})
	if _, err := s.AddSuperseding(Node{ID: "n1", Type: "fact", Title: "n1", Scope: ScopeGlobal}, "old"); err != nil {
		t.Fatalf("first supersede: %v", err)
	}
	first, _ := s.Get(ScopeGlobal, "old")
	time.Sleep(5 * time.Millisecond)
	if _, err := s.AddSuperseding(Node{ID: "n2", Type: "fact", Title: "n2", Scope: ScopeGlobal}, "old"); err != nil {
		t.Fatalf("second supersede: %v", err)
	}
	again, _ := s.Get(ScopeGlobal, "old")
	if !again.Updated.Equal(first.Updated) {
		t.Errorf("re-superseding must not bump Updated on the old node: %v vs %v", again.Updated, first.Updated)
	}
}

func TestAddUpsertRevivesRetiredNode(t *testing.T) {
	s := newTestStore(t)
	mustAdd(t, s, Node{ID: "r", Type: "fact", Title: "deploy checklist", Content: "deploy checklist", Scope: ScopeGlobal})
	if err := s.Retire(ScopeGlobal, "r"); err != nil {
		t.Fatalf("Retire: %v", err)
	}
	res := mustAdd(t, s, Node{ID: "r", Type: "fact", Title: "deploy checklist", Content: "deploy checklist v2", Scope: ScopeGlobal})
	if res.Created || !res.Revived {
		t.Errorf("re-Add of a retired node must report Revived, got %+v", res)
	}
	n, _ := s.Get(ScopeGlobal, "r")
	if n.ExpiredAt != nil || n.InvalidAt != nil {
		t.Error("revival must clear ExpiredAt and InvalidAt")
	}
	q, err := s.Query("deploy checklist", []string{ScopeGlobal}, "", 10, View{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(q) != 1 || q[0].ID != "r" {
		t.Errorf("revived node must be visible again, got %+v", q)
	}
}

func TestGetReturnsRetiredNodeUnfiltered(t *testing.T) {
	s := newTestStore(t)
	mustAdd(t, s, Node{ID: "r", Type: "note", Title: "r", Scope: ScopeGlobal})
	if err := s.Retire(ScopeGlobal, "r"); err != nil {
		t.Fatalf("Retire: %v", err)
	}
	n, err := s.Get(ScopeGlobal, "r")
	if err != nil {
		t.Fatalf("Get must return retired nodes: %v", err)
	}
	if n.ExpiredAt == nil {
		t.Error("returned node must carry its ExpiredAt stamp")
	}
}

func TestQueryAsOfReconstructsPastView(t *testing.T) {
	s := newTestStore(t)
	mustAdd(t, s, Node{ID: "era", Type: "fact", Title: "deploy checklist", Content: "deploy checklist", Scope: ScopeGlobal})

	// Backdate the lifecycle by hand: created day 1, invalidated day 10.
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	invalid := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	n, _ := s.Get(ScopeGlobal, "era")
	n.Created = created
	n.InvalidAt = &invalid
	s.mu.Lock()
	s.scopes[ScopeGlobal]["era"] = n
	if err := s.saveScopeLocked(ScopeGlobal); err != nil {
		s.mu.Unlock()
		t.Fatalf("save: %v", err)
	}
	s.mu.Unlock()

	query := func(at time.Time) int {
		res, err := s.Query("deploy checklist", []string{ScopeGlobal}, "", 10, View{AsOf: at})
		if err != nil {
			t.Fatalf("Query as_of %v: %v", at, err)
		}
		return len(res)
	}
	if got := query(created.Add(-time.Hour)); got != 0 {
		t.Errorf("before creation: want 0, got %d", got)
	}
	if got := query(created.AddDate(0, 0, 5)); got != 1 {
		t.Errorf("while valid: want 1, got %d", got)
	}
	if got := query(invalid); got != 0 {
		t.Errorf("at InvalidAt (half-open): want 0, got %d", got)
	}
	if got := query(invalid.AddDate(0, 0, 5)); got != 0 {
		t.Errorf("after invalidation: want 0, got %d", got)
	}
}

func TestIncludeExpiredReturnsEverything(t *testing.T) {
	s := newTestStore(t)
	mustAdd(t, s, Node{ID: "live", Type: "fact", Title: "deploy checklist", Content: "deploy checklist", Scope: ScopeGlobal})
	mustAdd(t, s, Node{ID: "dead", Type: "fact", Title: "deploy checklist", Content: "deploy checklist", Scope: ScopeGlobal})
	if err := s.Retire(ScopeGlobal, "dead"); err != nil {
		t.Fatalf("Retire: %v", err)
	}
	res, err := s.Query("deploy checklist", []string{ScopeGlobal}, "", 10, View{IncludeExpired: true})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) != 2 {
		t.Errorf("include_expired must return both nodes, got %d", len(res))
	}
	out, err := s.List(ScopeGlobal, "", 0, View{IncludeExpired: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("include_expired list must return both nodes, got %d", len(out))
	}
}

// TestIncludeExpiredRespectsAsOfExistence pins that IncludeExpired only
// bypasses the invalidated/expired checks, not the Created/ValidAt existence
// checks — combining as_of with include_expired must not resurrect a node
// that had not been created yet as of the queried instant.
func TestIncludeExpiredRespectsAsOfExistence(t *testing.T) {
	s := newTestStore(t)
	mustAdd(t, s, Node{ID: "recent", Type: "fact", Title: "deploy checklist", Content: "deploy checklist", Scope: ScopeGlobal})

	past := time.Now().UTC().AddDate(0, 0, -1)
	res, err := s.Query("deploy checklist", []string{ScopeGlobal}, "", 10, View{AsOf: past, IncludeExpired: true})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("include_expired must still respect as_of existence: node created after as_of must not appear, got %+v", res)
	}
	out, err := s.List(ScopeGlobal, "", 0, View{AsOf: past, IncludeExpired: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("include_expired list must still respect as_of existence, got %+v", out)
	}
}

func TestFutureValidAtHiddenFromCurrentView(t *testing.T) {
	s := newTestStore(t)
	future := time.Now().UTC().Add(24 * time.Hour)
	mustAdd(t, s, Node{ID: "embargo", Type: "fact", Title: "deploy checklist", Content: "deploy checklist", Scope: ScopeGlobal, ValidAt: &future})
	res, err := s.Query("deploy checklist", []string{ScopeGlobal}, "", 10, View{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("future-valid node must be hidden from the current view, got %d", len(res))
	}
	res, err = s.Query("deploy checklist", []string{ScopeGlobal}, "", 10, View{AsOf: future.Add(time.Hour)})
	if err != nil {
		t.Fatalf("Query as_of: %v", err)
	}
	if len(res) != 1 {
		t.Errorf("node must be visible at a time past its ValidAt, got %d", len(res))
	}
}

func TestOldScopeFileWithoutTemporalFieldsLoadsLive(t *testing.T) {
	dir := t.TempDir()
	// A pre-temporal schema v1 file: no valid_at/invalid_at/expired_at keys.
	raw := `{
  "schema_version": 1,
  "scope": "global",
  "nodes": [
    {"id": "legacy", "type": "fact", "title": "deploy checklist", "content": "deploy checklist",
     "scope": "global", "created": "2026-01-01T00:00:00Z", "updated": "2026-01-01T00:00:00Z"}
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "global.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	res, err := s.Query("deploy checklist", []string{ScopeGlobal}, "", 10, View{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) != 1 || res[0].ID != "legacy" {
		t.Errorf("legacy node must load as live, got %+v", res)
	}
	if err := json.Unmarshal([]byte(raw), &scopeFileV1{}); err != nil {
		t.Fatalf("fixture must be valid v1 JSON: %v", err)
	}
}

func TestQueryRecencyBoost(t *testing.T) {
	s := newTestStore(t)
	mustAdd(t, s, Node{ID: "stale", Type: "fact", Title: "deploy checklist", Content: "deploy checklist", Scope: ScopeGlobal})
	mustAdd(t, s, Node{ID: "fresh", Type: "fact", Title: "deploy checklist", Content: "deploy checklist", Scope: ScopeGlobal})

	// Backdate "stale" far past the half-life; identical text otherwise.
	old := time.Now().UTC().AddDate(-1, 0, 0)
	n, _ := s.Get(ScopeGlobal, "stale")
	n.Updated = old
	s.mu.Lock()
	s.scopes[ScopeGlobal]["stale"] = n
	if err := s.saveScopeLocked(ScopeGlobal); err != nil {
		s.mu.Unlock()
		t.Fatalf("save: %v", err)
	}
	s.mu.Unlock()

	res, err := s.Query("deploy checklist", []string{ScopeGlobal}, "", 10, View{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) != 2 || res[0].ID != "fresh" {
		t.Errorf("recency boost should rank the fresh node first, got %+v", res)
	}
}

func TestRecencyBoostIsBounded(t *testing.T) {
	s := newTestStore(t)
	// Strong match, ancient; weak match, fresh. BM25 gap must win.
	mustAdd(t, s, Node{ID: "canon", Type: "rule", Title: "docker bridge networking on mac", Content: "docker bridge networking on mac use port mapping", Scope: ScopeGlobal})
	mustAdd(t, s, Node{ID: "noise", Type: "note", Title: "misc notes", Content: "mentioned docker once among many unrelated words in a long note", Scope: ScopeGlobal})

	old := time.Now().UTC().AddDate(-2, 0, 0)
	n, _ := s.Get(ScopeGlobal, "canon")
	n.Updated = old
	s.mu.Lock()
	s.scopes[ScopeGlobal]["canon"] = n
	if err := s.saveScopeLocked(ScopeGlobal); err != nil {
		s.mu.Unlock()
		t.Fatalf("save: %v", err)
	}
	s.mu.Unlock()

	res, err := s.Query("docker bridge networking mac", []string{ScopeGlobal}, "", 10, View{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) == 0 || res[0].ID != "canon" {
		t.Errorf("a +10%%-max recency boost must not overturn a real BM25 gap, got %+v", res)
	}
}
