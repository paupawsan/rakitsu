package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func TestAddGetRoundtrip(t *testing.T) {
	s := newTestStore(t)
	res, err := s.Add(Node{Type: "gotcha", Title: "Viper lowercases keys", Content: "env keys come back lowercased", Scope: ScopeGlobal, Tags: []string{"viper", "config"}})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !res.Created {
		t.Fatal("expected Created=true on first Add")
	}
	n, err := s.Get(ScopeGlobal, "viper-lowercases-keys")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n.Priority != 5 {
		t.Errorf("default priority = %d, want 5", n.Priority)
	}
	if n.Created.IsZero() || n.Updated.IsZero() {
		t.Error("timestamps not set")
	}
}

func TestAddUpsertPreservesCreatedAndLinks(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Add(Node{ID: "a", Type: "fact", Title: "A", Content: "v1", Scope: ScopeGlobal}); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	if _, err := s.Add(Node{ID: "b", Type: "fact", Title: "B", Content: "x", Scope: ScopeGlobal}); err != nil {
		t.Fatalf("Add b: %v", err)
	}
	if err := s.AddLink(ScopeGlobal, "a", "b", "related_to", 0.7); err != nil {
		t.Fatalf("AddLink: %v", err)
	}
	first, _ := s.Get(ScopeGlobal, "a")

	res, err := s.Add(Node{ID: "a", Type: "fact", Title: "A", Content: "v2", Scope: ScopeGlobal})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if res.Created {
		t.Error("expected Created=false on upsert")
	}
	if res.Revived {
		t.Error("expected Revived=false on a live-node upsert")
	}
	n, _ := s.Get(ScopeGlobal, "a")
	if n.Content != "v2" {
		t.Errorf("content = %q, want v2", n.Content)
	}
	if !n.Created.Equal(first.Created) {
		t.Error("upsert must preserve Created")
	}
	if len(n.Links) != 1 || n.Links[0].Target != "b" {
		t.Errorf("upsert must preserve links, got %+v", n.Links)
	}
}

func TestInvalidTypeAndRelation(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Add(Node{Type: "vibe", Title: "x", Scope: ScopeGlobal}); err == nil {
		t.Error("expected error for invalid type")
	}
	if _, err := s.Add(Node{ID: "a", Type: "fact", Title: "A", Scope: ScopeGlobal}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.AddLink(ScopeGlobal, "a", "b", "loves", 1); err == nil {
		t.Error("expected error for invalid relation")
	}
	if err := s.AddLink(ScopeGlobal, "missing", "a", "related_to", 1); err == nil {
		t.Error("expected error for missing source")
	}
	// Dangling target is allowed by design.
	if err := s.AddLink(ScopeGlobal, "a", "not-written-yet", "related_to", 1); err != nil {
		t.Errorf("dangling target should be allowed: %v", err)
	}
}

func TestPersistenceAcrossStoreInstances(t *testing.T) {
	dir := t.TempDir()
	s1, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	scope := ProjectScope("rakitsu")
	if _, err := s1.Add(Node{ID: "p1", Type: "rule", Title: "Branch first", Content: "checkout -b before edits", Scope: scope}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	s2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore reopen: %v", err)
	}
	n, err := s2.Get(scope, "p1")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if n.Title != "Branch first" {
		t.Errorf("title = %q", n.Title)
	}
	// Scope file name is sanitized and derived from the scope.
	if _, err := os.Stat(filepath.Join(dir, "project-rakitsu.json")); err != nil {
		t.Errorf("expected scope file project-rakitsu.json: %v", err)
	}
}

func TestQueryRankingAndScopes(t *testing.T) {
	s := newTestStore(t)
	proj := ProjectScope("rakitsu")
	seed := []Node{
		{ID: "docker-net", Type: "gotcha", Title: "Docker Desktop host networking on Mac", Content: "use bridge plus port mapping instead of host network", Scope: proj, Tags: []string{"docker", "mac"}},
		{ID: "gpg-sign", Type: "procedure", Title: "GPG signing via 1Password", Content: "preset the git sign passphrase from terminal", Scope: proj},
		{ID: "docker-global", Type: "gotcha", Title: "Docker compose needs --build", Content: "stale image without --build flag in docker compose", Scope: ScopeGlobal, Tags: []string{"docker"}},
	}
	for _, n := range seed {
		if _, err := s.Add(n); err != nil {
			t.Fatalf("Add %s: %v", n.ID, err)
		}
	}

	res, err := s.Query("docker network mac", []string{proj, ScopeGlobal}, "", 10, View{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) < 2 {
		t.Fatalf("expected >=2 results, got %d", len(res))
	}
	if res[0].ID != "docker-net" {
		t.Errorf("top hit = %s, want docker-net", res[0].ID)
	}
	for _, r := range res {
		if r.ID == "gpg-sign" {
			t.Error("gpg-sign must not match a docker query")
		}
	}

	// Type filter.
	res, err = s.Query("docker", []string{proj, ScopeGlobal}, "procedure", 10, View{})
	if err != nil {
		t.Fatalf("Query typed: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("procedure-typed docker query should be empty, got %d", len(res))
	}
}

func TestQueryPriorityBoost(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Add(Node{ID: "low", Type: "fact", Title: "deploy checklist", Content: "deploy checklist", Scope: ScopeGlobal, Priority: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(Node{ID: "high", Type: "fact", Title: "deploy checklist", Content: "deploy checklist", Scope: ScopeGlobal, Priority: 9}); err != nil {
		t.Fatal(err)
	}
	res, err := s.Query("deploy checklist", []string{ScopeGlobal}, "", 10, View{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) != 2 || res[0].ID != "high" {
		t.Errorf("priority boost should rank 'high' first, got %+v", res)
	}
}

func TestListNewestFirstAndLimit(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"one", "two", "three"} {
		if _, err := s.Add(Node{ID: id, Type: "note", Title: id, Content: id, Scope: ScopeGlobal}); err != nil {
			t.Fatal(err)
		}
	}
	// Touch "one" so it becomes newest.
	if _, err := s.Add(Node{ID: "one", Type: "note", Title: "one", Content: "updated", Scope: ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	out, err := s.List(ScopeGlobal, "", 2, View{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 2 || out[0].ID != "one" {
		t.Errorf("List = %+v, want one first with limit 2", out)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Add(Node{ID: "gone", Type: "note", Title: "gone", Scope: ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ScopeGlobal, "gone"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ScopeGlobal, "gone"); err == nil {
		t.Error("expected Get to fail after Delete")
	}
	if err := s.Delete(ScopeGlobal, "gone"); err == nil {
		t.Error("expected Delete of missing node to fail")
	}
}

func TestSlugify(t *testing.T) {
	if got := Slugify("Viper lowercases KEYS!"); got != "viper-lowercases-keys" {
		t.Errorf("Slugify = %q", got)
	}
	long := Slugify(strings.Repeat("word ", 40))
	if len(long) > 80 {
		t.Errorf("slug too long: %d", len(long))
	}
}

func TestScopeHelpers(t *testing.T) {
	if ProjectScope("") != ScopeGlobal {
		t.Error("empty project must map to global")
	}
	if ProjectScope("x") != "project:x" || SessionScope("y") != "session:y" {
		t.Error("scope helpers wrong")
	}
}

func TestQueryEdgeBoost(t *testing.T) {
	s := newTestStore(t)
	// Three equal matches; a and b are linked, c is an island. The edge
	// boost must lift both link endpoints above the unlinked node.
	for _, id := range []string{"a", "b", "c"} {
		if _, err := s.Add(Node{ID: id, Type: "fact", Title: "deploy checklist", Content: "deploy checklist", Scope: ScopeGlobal}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddLink(ScopeGlobal, "a", "b", "related_to", 1.0); err != nil {
		t.Fatalf("AddLink: %v", err)
	}
	res, err := s.Query("deploy checklist", []string{ScopeGlobal}, "", 10, View{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("expected 3 results, got %d", len(res))
	}
	if res[2].ID != "c" {
		t.Errorf("unlinked node should rank last, got order %s,%s,%s", res[0].ID, res[1].ID, res[2].ID)
	}
	if res[0].Score <= res[2].Score {
		t.Errorf("linked nodes should outscore the island: %v vs %v", res[0].Score, res[2].Score)
	}
}

// TestOpenSharedNormalizesDir: two callers reaching OpenShared with
// different-but-equivalent spellings of the same directory ("" vs its
// expanded default) must share one Store instance, not race two separate
// ones through the same on-disk scope files.
func TestOpenSharedNormalizesDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaultDir, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}

	s1, err := OpenShared("")
	if err != nil {
		t.Fatalf(`OpenShared(""): %v`, err)
	}
	s2, err := OpenShared(defaultDir)
	if err != nil {
		t.Fatalf("OpenShared(%q): %v", defaultDir, err)
	}
	if s1 != s2 {
		t.Fatal(`OpenShared("") and OpenShared(<resolved default dir>) returned different instances for the same directory`)
	}
}

// TestGetListQueryReturnIndependentCopies: a caller mutating a Tags/Links
// slice on a node returned by Get, List, or Query must not reach into the
// store's own copy — those slices used to alias the same backing array.
func TestGetListQueryReturnIndependentCopies(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Add(Node{ID: "n1", Type: "rule", Title: "t", Content: "searchable content", Scope: ScopeGlobal, Tags: []string{"original"}}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	assertUnmutated := func(t *testing.T, label string) {
		t.Helper()
		n, err := s.Get(ScopeGlobal, "n1")
		if err != nil {
			t.Fatalf("Get after %s mutation: %v", label, err)
		}
		if n.Tags[0] != "original" {
			t.Errorf("store Tags mutated via %s copy: got %q", label, n.Tags[0])
		}
	}

	got, err := s.Get(ScopeGlobal, "n1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got.Tags[0] = "MUTATED-VIA-GET"
	assertUnmutated(t, "Get")

	list, err := s.List(ScopeGlobal, "", 0, View{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	list[0].Tags[0] = "MUTATED-VIA-LIST"
	assertUnmutated(t, "List")

	scored, err := s.Query("searchable content", []string{ScopeGlobal}, "", 10, View{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	scored[0].Tags[0] = "MUTATED-VIA-QUERY"
	assertUnmutated(t, "Query")
}
