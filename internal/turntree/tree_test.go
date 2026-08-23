package turntree

import (
	"encoding/json"
	"errors"
	"testing"
)

// fakeEntry is a minimal entry type used by tests. Real consumers use
// transcriptEntry / ContentBlock; the package is agnostic.
type fakeEntry struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// pathIDs returns the IDs of nodes on the active path for compact asserts.
func pathIDs[E any](t *Tree[E]) []string {
	path := t.ActivePath()
	out := make([]string, len(path))
	for i, n := range path {
		out[i] = n.ID
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNewIsEmpty(t *testing.T) {
	tr := New[fakeEntry]()
	if got := tr.ActivePath(); got != nil {
		t.Fatalf("ActivePath on empty tree = %v, want nil", got)
	}
	if got := tr.ActiveLeaf(); got != nil {
		t.Fatalf("ActiveLeaf on empty tree = %v, want nil", got)
	}
	if _, ok := tr.Get("anything"); ok {
		t.Fatalf("Get on empty tree returned ok")
	}
}

func TestAppendTurnRoot(t *testing.T) {
	tr := New[fakeEntry]()
	node, err := tr.AppendTurn("t1", "hello", "")
	if err != nil {
		t.Fatalf("AppendTurn root: %v", err)
	}
	if node.Status != StatusGenerating {
		t.Fatalf("new node status = %q, want %q", node.Status, StatusGenerating)
	}
	if node.UserText != "hello" {
		t.Fatalf("UserText = %q, want %q", node.UserText, "hello")
	}
	if !eq(tr.Roots, []string{"t1"}) {
		t.Fatalf("Roots = %v, want [t1]", tr.Roots)
	}
	if got := tr.ActiveChild[""]; got != "t1" {
		t.Fatalf("ActiveChild[''] = %q, want t1", got)
	}
	if !eq(pathIDs(tr), []string{"t1"}) {
		t.Fatalf("ActivePath = %v, want [t1]", pathIDs(tr))
	}
	if tr.ActiveLeaf().ID != "t1" {
		t.Fatalf("ActiveLeaf().ID = %q, want t1", tr.ActiveLeaf().ID)
	}
}

func TestAppendTurnChain(t *testing.T) {
	tr := New[fakeEntry]()
	if _, err := tr.AppendTurn("t1", "a", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.AppendTurn("t2", "b", "t1"); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.AppendTurn("t3", "c", "t2"); err != nil {
		t.Fatal(err)
	}
	if !eq(pathIDs(tr), []string{"t1", "t2", "t3"}) {
		t.Fatalf("ActivePath = %v, want [t1 t2 t3]", pathIDs(tr))
	}
	// Each parent's Children list is in append order.
	t1, _ := tr.Get("t1")
	if !eq(t1.Children, []string{"t2"}) {
		t.Fatalf("t1.Children = %v, want [t2]", t1.Children)
	}
}

func TestAddSiblingActivatesNew(t *testing.T) {
	tr := New[fakeEntry]()
	_, _ = tr.AppendTurn("t1", "a", "")
	_, _ = tr.AppendTurn("t2", "b", "t1")
	// Edit t2 → sibling t2b under t1.
	sib, err := tr.AddSibling("t2b", "t2", "b-edited")
	if err != nil {
		t.Fatalf("AddSibling: %v", err)
	}
	if sib.ParentID != "t1" {
		t.Fatalf("new sibling ParentID = %q, want t1", sib.ParentID)
	}
	// t1.Children now has both, sibling order preserved.
	t1, _ := tr.Get("t1")
	if !eq(t1.Children, []string{"t2", "t2b"}) {
		t.Fatalf("t1.Children = %v, want [t2 t2b]", t1.Children)
	}
	// Active path follows the new sibling.
	if !eq(pathIDs(tr), []string{"t1", "t2b"}) {
		t.Fatalf("ActivePath after edit = %v, want [t1 t2b]", pathIDs(tr))
	}
}

func TestSwitchToSiblingCyclesBothDirections(t *testing.T) {
	tr := New[fakeEntry]()
	_, _ = tr.AppendTurn("t1", "a", "")
	_, _ = tr.AppendTurn("t2a", "b1", "t1")
	_, _ = tr.AddSibling("t2b", "t2a", "b2")
	_, _ = tr.AddSibling("t2c", "t2a", "b3")
	// Active is t2c (last added).
	if tr.ActiveChild["t1"] != "t2c" {
		t.Fatalf("ActiveChild[t1] = %q, want t2c", tr.ActiveChild["t1"])
	}
	// +1 from t2c wraps to t2a.
	got, err := tr.SwitchToSibling("t2c", +1)
	if err != nil {
		t.Fatal(err)
	}
	if got != "t2a" {
		t.Fatalf("switch +1 from t2c = %q, want t2a", got)
	}
	// -1 from t2a wraps to t2c.
	got, err = tr.SwitchToSibling("t2a", -1)
	if err != nil {
		t.Fatal(err)
	}
	if got != "t2c" {
		t.Fatalf("switch -1 from t2a = %q, want t2c", got)
	}
	// Larger step still cycles.
	got, _ = tr.SwitchToSibling("t2c", +5) // (2+5)%3 = 1 → t2b
	if got != "t2b" {
		t.Fatalf("switch +5 from t2c = %q, want t2b", got)
	}
}

func TestSwitchToSiblingSingleChild(t *testing.T) {
	tr := New[fakeEntry]()
	_, _ = tr.AppendTurn("t1", "a", "")
	_, _ = tr.AppendTurn("t2", "b", "t1")
	id, err := tr.SwitchToSibling("t2", +1)
	if !errors.Is(err, ErrSiblingCount) {
		t.Fatalf("err = %v, want ErrSiblingCount", err)
	}
	if id != "t2" {
		t.Fatalf("id = %q, want t2 (no-op)", id)
	}
}

func TestSetActivePathThroughMultipleBranches(t *testing.T) {
	// Build:
	//   t1 ─┬─ t2a ── t3a
	//       └─ t2b ── t3b
	// Active by construction ends at t3b. SetActivePath("t3a") flips it.
	tr := New[fakeEntry]()
	_, _ = tr.AppendTurn("t1", "a", "")
	_, _ = tr.AppendTurn("t2a", "b1", "t1")
	_, _ = tr.AppendTurn("t3a", "c1", "t2a")
	_, _ = tr.AddSibling("t2b", "t2a", "b2")
	_, _ = tr.AppendTurn("t3b", "c2", "t2b")
	if !eq(pathIDs(tr), []string{"t1", "t2b", "t3b"}) {
		t.Fatalf("ActivePath = %v, want [t1 t2b t3b]", pathIDs(tr))
	}
	if err := tr.SetActivePath("t3a"); err != nil {
		t.Fatal(err)
	}
	if !eq(pathIDs(tr), []string{"t1", "t2a", "t3a"}) {
		t.Fatalf("ActivePath after SetActivePath(t3a) = %v, want [t1 t2a t3a]", pathIDs(tr))
	}
	// SetActivePath on the root works too.
	if err := tr.SetActivePath("t1"); err != nil {
		t.Fatal(err)
	}
	// Path falls through to first child of t1 (t2a, which we activated), then its first child t3a.
	if !eq(pathIDs(tr), []string{"t1", "t2a", "t3a"}) {
		t.Fatalf("ActivePath after SetActivePath(t1) = %v, want [t1 t2a t3a]", pathIDs(tr))
	}
}

func TestSiblingIndex(t *testing.T) {
	tr := New[fakeEntry]()
	_, _ = tr.AppendTurn("t1", "a", "")
	_, _ = tr.AppendTurn("t2a", "b1", "t1")
	_, _ = tr.AddSibling("t2b", "t2a", "b2")
	_, _ = tr.AddSibling("t2c", "t2a", "b3")

	cases := []struct {
		id             string
		wantIdx, wantN int
	}{
		{"t2a", 0, 3},
		{"t2b", 1, 3},
		{"t2c", 2, 3},
		{"t1", 0, 1},
		{"missing", -1, 0},
	}
	for _, c := range cases {
		idx, n := tr.SiblingIndex(c.id)
		if idx != c.wantIdx || n != c.wantN {
			t.Errorf("SiblingIndex(%q) = (%d, %d), want (%d, %d)", c.id, idx, n, c.wantIdx, c.wantN)
		}
	}
}

func TestAppendEntryAndFinalize(t *testing.T) {
	tr := New[fakeEntry]()
	_, _ = tr.AppendTurn("t1", "a", "")
	if err := tr.AppendEntry("t1", fakeEntry{Kind: "assistant", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := tr.AppendEntry("t1", fakeEntry{Kind: "tool", Text: "ls"}); err != nil {
		t.Fatal(err)
	}
	node, _ := tr.Get("t1")
	if len(node.Entries) != 2 {
		t.Fatalf("Entries len = %d, want 2", len(node.Entries))
	}
	if err := tr.Finalize("t1", StatusComplete); err != nil {
		t.Fatal(err)
	}
	if node.Status != StatusComplete {
		t.Fatalf("Status = %q, want %q", node.Status, StatusComplete)
	}
	if err := tr.AppendEntry("missing", fakeEntry{}); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("AppendEntry on unknown: err = %v, want ErrUnknownNode", err)
	}
	if err := tr.Finalize("missing", StatusComplete); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("Finalize on unknown: err = %v, want ErrUnknownNode", err)
	}
}

func TestErrors(t *testing.T) {
	tr := New[fakeEntry]()
	if _, err := tr.AppendTurn("", "x", ""); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("empty id err = %v, want ErrEmptyID", err)
	}
	if _, err := tr.AppendTurn("t1", "x", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.AppendTurn("t1", "x", ""); !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("dup err = %v, want ErrDuplicateID", err)
	}
	if _, err := tr.AppendTurn("t2", "x", "missing"); !errors.Is(err, ErrUnknownParent) {
		t.Fatalf("unknown parent err = %v, want ErrUnknownParent", err)
	}
	if _, err := tr.AddSibling("t3", "missing", "x"); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("AddSibling unknown target err = %v, want ErrUnknownNode", err)
	}
	if _, err := tr.SwitchToSibling("missing", +1); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("SwitchToSibling unknown err = %v, want ErrUnknownNode", err)
	}
	if err := tr.SetActivePath("missing"); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("SetActivePath unknown err = %v, want ErrUnknownNode", err)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	tr := New[fakeEntry]()
	_, _ = tr.AppendTurn("t1", "a", "")
	_, _ = tr.AppendTurn("t2a", "b1", "t1")
	_ = tr.AppendEntry("t2a", fakeEntry{Kind: "assistant", Text: "first"})
	_ = tr.Finalize("t2a", StatusComplete)
	_, _ = tr.AddSibling("t2b", "t2a", "b2")
	_ = tr.AppendEntry("t2b", fakeEntry{Kind: "assistant", Text: "second"})
	_ = tr.Finalize("t2b", StatusComplete)
	// Active by construction = t2b.

	data, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	loaded, err := LoadFromJSON[fakeEntry](data)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !eq(pathIDs(loaded), []string{"t1", "t2b"}) {
		t.Fatalf("loaded ActivePath = %v, want [t1 t2b]", pathIDs(loaded))
	}
	if loaded.Nodes["t2a"].Entries[0].Text != "first" {
		t.Fatalf("loaded t2a entries lost text")
	}
	// Sibling switch round-trips.
	if _, err := loaded.SwitchToSibling("t2b", -1); err != nil {
		t.Fatal(err)
	}
	if !eq(pathIDs(loaded), []string{"t1", "t2a"}) {
		t.Fatalf("after switch on loaded tree: %v, want [t1 t2a]", pathIDs(loaded))
	}
}

func TestRetryContextRoundTrip(t *testing.T) {
	// A turn produced by an agent-driven self-correction carries a
	// RetryCtx referencing the failed sibling. Ordinary turns omit it.
	tr := New[fakeEntry]()
	_, _ = tr.AppendTurn("t1", "do the thing", "")
	_, _ = tr.AddSibling("t1b", "t1", "do the thing") // auto-regenerate sibling
	tr.Nodes["t1b"].RetryCtx = &RetryContext{
		Source:      "rollback",
		PriorTurnID: "t1",
		Reason:      "prior attempt produced no final answer",
	}

	data, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Ordinary node serializes without the field (omitempty).
	if got := string(data); contains(got, `"id":"t1",`) && contains(got, `"retry_ctx"`) {
		// If retry_ctx appears alongside t1's id we'd want to verify it's
		// only on t1b — easiest sanity check is the field must appear
		// exactly once (for t1b).
		if countSubstr(got, `"retry_ctx"`) != 1 {
			t.Fatalf("retry_ctx leaked onto ordinary nodes: %s", got)
		}
	}

	loaded, err := LoadFromJSON[fakeEntry](data)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Nodes["t1"].RetryCtx != nil {
		t.Fatalf("ordinary turn has RetryCtx after round-trip: %+v", loaded.Nodes["t1"].RetryCtx)
	}
	rc := loaded.Nodes["t1b"].RetryCtx
	if rc == nil {
		t.Fatal("RetryCtx lost after round-trip")
	}
	if rc.Source != "rollback" || rc.PriorTurnID != "t1" || rc.Reason == "" {
		t.Fatalf("RetryCtx round-trip mangled fields: %+v", rc)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOfSubstr(s, sub) >= 0
}

func indexOfSubstr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func countSubstr(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); {
		if s[i:i+len(sub)] == sub {
			n++
			i += len(sub)
		} else {
			i++
		}
	}
	return n
}

func TestActivePathFallthroughOnUnsetActiveChild(t *testing.T) {
	// Simulate a tree loaded from older state where ActiveChild was not
	// recorded for some node. The first child is taken as a fallback so
	// the path still descends to a leaf.
	tr := New[fakeEntry]()
	_, _ = tr.AppendTurn("t1", "a", "")
	_, _ = tr.AppendTurn("t2", "b", "t1")
	_, _ = tr.AppendTurn("t3", "c", "t2")
	// Wipe ActiveChild for t1 and t2.
	delete(tr.ActiveChild, "t1")
	delete(tr.ActiveChild, "t2")
	if !eq(pathIDs(tr), []string{"t1", "t2", "t3"}) {
		t.Fatalf("fallback path = %v, want [t1 t2 t3]", pathIDs(tr))
	}
}

func TestActiveLeafGenerating(t *testing.T) {
	tr := New[fakeEntry]()
	_, _ = tr.AppendTurn("t1", "a", "")
	if leaf := tr.ActiveLeaf(); leaf == nil || leaf.ID != "t1" || leaf.Status != StatusGenerating {
		t.Fatalf("ActiveLeaf = %+v, want id=t1 status=generating", leaf)
	}
	_ = tr.Finalize("t1", StatusComplete)
	if tr.ActiveLeaf().Status != StatusComplete {
		t.Fatalf("after Finalize, leaf status = %q", tr.ActiveLeaf().Status)
	}
}

func TestHydrate(t *testing.T) {
	// Tree with nil maps from a struct literal.
	tr := &Tree[fakeEntry]{}
	tr.Hydrate()
	if tr.Nodes == nil || tr.ActiveChild == nil {
		t.Fatal("Hydrate left a nil map")
	}
	// Idempotent.
	tr.Hydrate()
	if len(tr.Nodes) != 0 || len(tr.ActiveChild) != 0 {
		t.Fatal("second Hydrate mutated content")
	}
}
