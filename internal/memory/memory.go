// Package memory provides rakitsu's native, file-backed memory and knowledge
// graph: typed knowledge nodes (rule, pattern, fact, procedure, gotcha, note,
// summary) with tags, priorities, and typed links, organized into scopes
// ("global", "project:<id>", "session:<id>") and recalled via BM25 ranking.
//
// Knowledge is bi-temporal: nodes carry valid_at/invalid_at/expired_at
// stamps, superseding or retiring hides a node from the default current view
// without destroying it, and Query/List accept a View for as-of time travel.
//
// Design constraints:
//   - single binary, no external DB, no CGO: one JSON file per scope under
//     ~/.rakitsu/memory/, atomic tmp+rename writes (the SaveChatTree pattern)
//   - leaf package: imports stdlib + internal/llm only, so tools, chat
//     surfaces, and (later) validators can all depend on it without cycles
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Scope constants. Project and session scopes are produced by ProjectScope /
// SessionScope; ScopeGlobal is shared across all projects.
const ScopeGlobal = "global"

// Node types. "summary" is reserved for rolling conversation summaries
// (conversation.go); the rest mirror the knowledge-graph taxonomy.
var validTypes = map[string]bool{
	"rule": true, "pattern": true, "fact": true, "procedure": true,
	"gotcha": true, "note": true, "summary": true,
}

// Link relations.
var validRelations = map[string]bool{
	"applies_to": true, "depends_on": true, "part_of": true,
	"related_to": true, "supersedes": true, "contradicts": true,
}

// Link is a typed edge from the owning Node to Target (a node ID, possibly in
// another scope). Dangling targets are permitted — a link to a node that does
// not exist yet marks knowledge worth writing, not an error.
type Link struct {
	Target   string  `json:"target"`
	Relation string  `json:"relation"`
	Weight   float64 `json:"weight,omitempty"`
}

// Node is one unit of knowledge.
type Node struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags,omitempty"`
	Priority int      `json:"priority,omitempty"` // 1-10, default 5
	Scope    string   `json:"scope"`
	Source   string   `json:"source,omitempty"`
	Links    []Link   `json:"links,omitempty"`
	// Bi-temporal lifecycle (nil = live). ValidAt/InvalidAt bound when the
	// knowledge is true in the world; ExpiredAt records retirement from the
	// working set. Additive to schema v1: files written before these fields
	// existed load unchanged (nil = live). An older binary that rewrites a
	// scope file drops them — acceptable pre-release.
	ValidAt   *time.Time `json:"valid_at,omitempty"`
	InvalidAt *time.Time `json:"invalid_at,omitempty"`
	ExpiredAt *time.Time `json:"expired_at,omitempty"`
	Created   time.Time  `json:"created"`
	Updated   time.Time  `json:"updated"`
}

// View selects which temporal slice of the store Query and List see. The
// zero value is the current view: nodes that exist now, have become valid,
// and are neither invalidated (superseded) nor expired (retired). AsOf
// reconstructs what was known and true at that instant; IncludeExpired
// returns everything regardless of lifecycle.
type View struct {
	AsOf           time.Time
	IncludeExpired bool
}

// at returns the view's reference time.
func (v View) at() time.Time {
	if v.AsOf.IsZero() {
		return time.Now().UTC()
	}
	return v.AsOf
}

// At returns the view's reference time: AsOf when set, otherwise now.
// Exported so callers outside this package (the memory tool layer) can
// render lifecycle status against the same instant a query was run for,
// instead of wall-clock now.
func (v View) At() time.Time {
	return v.at()
}

// visible reports whether n belongs to the view at time t: the record
// existed and had become valid as of t, and — unless IncludeExpired — was
// neither invalidated nor expired. IncludeExpired only bypasses the
// invalidated/expired checks; it must never resurrect a node that did not
// exist yet at t, or as_of+include_expired would ignore the as_of bound
// entirely. Intervals are half-open — a node is visible at its ValidAt
// instant and hidden at its InvalidAt/ExpiredAt instant.
func (v View) visible(n *Node, t time.Time) bool {
	if n.Created.After(t) {
		return false
	}
	if n.ValidAt != nil && n.ValidAt.After(t) {
		return false
	}
	if v.IncludeExpired {
		return true
	}
	if n.InvalidAt != nil && !n.InvalidAt.After(t) {
		return false
	}
	if n.ExpiredAt != nil && !n.ExpiredAt.After(t) {
		return false
	}
	return true
}

// AddResult reports what an upsert did. Revived is true when the node
// existed but was invalidated or expired and the write brought it back into
// the current view.
type AddResult struct {
	Created bool
	Revived bool
}

// Scored is a Query result.
type Scored struct {
	Node
	Score float64 `json:"score"`
}

// scopeFileV1 is the on-disk format: one file per scope.
type scopeFileV1 struct {
	SchemaVersion int     `json:"schema_version"`
	Scope         string  `json:"scope"`
	Nodes         []*Node `json:"nodes"`
}

// ProjectScope returns the scope string for a project ID, falling back to
// ScopeGlobal when the config has no project_id.
func ProjectScope(projectID string) string {
	if projectID == "" {
		return ScopeGlobal
	}
	return "project:" + projectID
}

// SessionScope returns the scope string for a session ID.
func SessionScope(sessionID string) string {
	return "session:" + sessionID
}

// DefaultDir returns ~/.rakitsu/memory.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home dir: %w", err)
	}
	return filepath.Join(home, ".rakitsu", "memory"), nil
}

// Store is a file-backed memory store. One Store instance should be shared
// per process; scope files are loaded lazily and written atomically. Across
// processes the last writer wins per scope file — acceptable for Phase 1.
type Store struct {
	dir    string
	mu     sync.Mutex
	scopes map[string]map[string]*Node // scope -> node ID -> node
}

// sharedStores caches one Store per resolved dir string so every consumer in
// the process — per-agent tool sets, conversation memory, the serve daemon's
// chat sessions — shares the same mutex-guarded instance instead of racing
// whole-file rewrites through separate instances.
var (
	sharedMu     sync.Mutex
	sharedStores = map[string]*Store{}
)

// resolveDir expands dir to the canonical on-disk path NewStore would open:
// "" selects DefaultDir(), a leading "~/" is expanded against the user's
// home directory, and anything else is filepath.Clean'd. Callers that key a
// cache by dir (OpenShared) must resolve first, or different-but-equivalent
// spellings of the same directory collide silently instead of sharing state.
func resolveDir(dir string) (string, error) {
	if dir == "" {
		return DefaultDir()
	}
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand %q: %w", dir, err)
		}
		return filepath.Join(home, dir[2:]), nil
	}
	return filepath.Clean(dir), nil
}

// OpenShared returns the process-wide Store for dir (creating it on first
// use). Prefer this over NewStore outside of tests.
func OpenShared(dir string) (*Store, error) {
	resolved, err := resolveDir(dir)
	if err != nil {
		return nil, err
	}
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if s, ok := sharedStores[resolved]; ok {
		return s, nil
	}
	s, err := NewStore(resolved)
	if err != nil {
		return nil, err
	}
	sharedStores[resolved] = s
	return s, nil
}

// NewStore opens (creating if needed) a memory store rooted at dir. An empty
// dir selects DefaultDir(). A leading "~/" is expanded.
func NewStore(dir string) (*Store, error) {
	dir, err := resolveDir(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create memory dir %q: %w", dir, err)
	}
	return &Store{dir: dir, scopes: make(map[string]map[string]*Node)}, nil
}

// Dir returns the store's root directory.
func (s *Store) Dir() string { return s.dir }

var scopeSanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// scopePath maps a scope string to its JSON file.
// "global" -> global.json; "project:<id>" -> project-<id>.json; etc.
func (s *Store) scopePath(scope string) string {
	name := scope
	if i := strings.IndexByte(scope, ':'); i >= 0 {
		name = scope[:i] + "-" + scope[i+1:]
	}
	return filepath.Join(s.dir, scopeSanitizeRe.ReplaceAllString(name, "-")+".json")
}

// loadScopeLocked lazily loads a scope file. Caller holds s.mu.
func (s *Store) loadScopeLocked(scope string) (map[string]*Node, error) {
	if nodes, ok := s.scopes[scope]; ok {
		return nodes, nil
	}
	nodes := make(map[string]*Node)
	raw, err := os.ReadFile(s.scopePath(scope))
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("cannot read memory scope %q: %w", scope, err)
		}
	} else {
		var f scopeFileV1
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("corrupt memory scope file %q: %w", s.scopePath(scope), err)
		}
		for _, n := range f.Nodes {
			if n != nil && n.ID != "" {
				nodes[n.ID] = n
			}
		}
	}
	s.scopes[scope] = nodes
	return nodes, nil
}

// saveScopeLocked atomically persists one scope. Caller holds s.mu.
func (s *Store) saveScopeLocked(scope string) error {
	nodes := s.scopes[scope]
	list := make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		list = append(list, n)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	raw, err := json.MarshalIndent(scopeFileV1{SchemaVersion: 1, Scope: scope, Nodes: list}, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal memory scope %q: %w", scope, err)
	}
	path := s.scopePath(scope)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("cannot write memory scope %q: %w", scope, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) //nolint:errcheck
		return fmt.Errorf("cannot commit memory scope %q: %w", scope, err)
	}
	return nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify derives a node ID from free text: lowercase, non-alphanumerics
// collapsed to single hyphens.
func Slugify(text string) string {
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(text), "-"), "-")
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-")
	}
	return slug
}

// normalizeNode validates n and fills defaults (slugified ID, priority 5
// clamped to 1-10).
func normalizeNode(n *Node) error {
	if !validTypes[n.Type] {
		return fmt.Errorf("invalid node type %q (want rule|pattern|fact|procedure|gotcha|note|summary)", n.Type)
	}
	if n.Scope == "" {
		return fmt.Errorf("scope is required")
	}
	if n.ID == "" {
		n.ID = Slugify(n.Title)
	}
	if n.ID == "" {
		return fmt.Errorf("node needs an id or a title to derive one from")
	}
	if n.Priority == 0 {
		n.Priority = 5
	} else if n.Priority < 1 {
		n.Priority = 1
	} else if n.Priority > 10 {
		n.Priority = 10
	}
	return nil
}

// upsertLocked inserts or updates n in nodes. An update keeps the previous
// Created timestamp, Links (when the incoming node has none), and ValidAt
// (when the incoming node's is nil). Add asserts current truth: any
// InvalidAt/ExpiredAt on the previous version is cleared, reviving nodes
// that were superseded or retired. Caller holds s.mu.
func upsertLocked(nodes map[string]*Node, n Node, now time.Time) AddResult {
	res := AddResult{Created: true}
	if prev, ok := nodes[n.ID]; ok {
		res.Created = false
		res.Revived = prev.InvalidAt != nil || prev.ExpiredAt != nil
		n.Created = prev.Created
		if len(n.Links) == 0 {
			n.Links = prev.Links
		}
		if n.ValidAt == nil {
			n.ValidAt = prev.ValidAt
		}
	} else {
		n.Created = now
	}
	n.InvalidAt, n.ExpiredAt = nil, nil
	n.Updated = now
	nodes[n.ID] = &n
	return res
}

// Add upserts a node. A missing ID is slugified from the title; a missing
// priority defaults to 5 (clamped to 1-10). Upserting an existing ID keeps
// its Created timestamp and Links, and revives it if it was superseded or
// retired.
func (s *Store) Add(n Node) (AddResult, error) {
	if err := normalizeNode(&n); err != nil {
		return AddResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes, err := s.loadScopeLocked(n.Scope)
	if err != nil {
		return AddResult{}, err
	}
	res := upsertLocked(nodes, n, time.Now().UTC())
	return res, s.saveScopeLocked(n.Scope)
}

// AddSuperseding upserts n and invalidates the node oldID in the same scope,
// linking the new node to the old with a supersedes edge — the enforced way
// to replace knowledge instead of editing it. The old node keeps its
// earliest InvalidAt (re-superseding does not move the stamp) and stays
// reachable via Get, include-expired reads, and as-of views. Fails when
// oldID does not exist: a supersede that silently invalidates nothing is the
// failure mode this method exists to prevent.
func (s *Store) AddSuperseding(n Node, oldID string) (AddResult, error) {
	if err := normalizeNode(&n); err != nil {
		return AddResult{}, err
	}
	if n.ID == oldID {
		return AddResult{}, fmt.Errorf("node %q cannot supersede itself", n.ID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes, err := s.loadScopeLocked(n.Scope)
	if err != nil {
		return AddResult{}, err
	}
	old, ok := nodes[oldID]
	if !ok {
		return AddResult{}, fmt.Errorf("supersede target %q not found in scope %q", oldID, n.Scope)
	}
	now := time.Now().UTC()
	res := upsertLocked(nodes, n, now)
	if old.InvalidAt == nil {
		invalidAt := now
		old.InvalidAt = &invalidAt
		old.Updated = now
	}
	cur := nodes[n.ID]
	hasLink := false
	for i := range cur.Links {
		if cur.Links[i].Target == oldID && cur.Links[i].Relation == "supersedes" {
			cur.Links[i].Weight = 1.0
			hasLink = true
			break
		}
	}
	if !hasLink {
		cur.Links = append(cur.Links, Link{Target: oldID, Relation: "supersedes", Weight: 1.0})
	}
	return res, s.saveScopeLocked(n.Scope)
}

// Retire soft-deletes a node: it disappears from current-view Query and List
// but keeps its content, links, and history for Get, include-expired reads,
// and as-of views. Never destructive — a later Add on the same ID revives
// it. Retiring an already-retired node is an error.
func (s *Store) Retire(scope, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes, err := s.loadScopeLocked(scope)
	if err != nil {
		return err
	}
	n, ok := nodes[id]
	if !ok {
		return fmt.Errorf("node %q not found in scope %q", id, scope)
	}
	if n.ExpiredAt != nil {
		return fmt.Errorf("node %q already retired at %s", id, n.ExpiredAt.Format(time.RFC3339))
	}
	now := time.Now().UTC()
	n.ExpiredAt = &now
	n.Updated = now
	return s.saveScopeLocked(scope)
}

// cloneNode returns a value copy of n with its slice fields deep-copied, so
// a caller holding or mutating the result can't reach into the store's
// internal node without holding s.mu.
func cloneNode(n *Node) Node {
	cp := *n
	if n.Tags != nil {
		cp.Tags = append([]string(nil), n.Tags...)
	}
	if n.Links != nil {
		cp.Links = append([]Link(nil), n.Links...)
	}
	return cp
}

// Get returns a copy of one node.
func (s *Store) Get(scope, id string) (*Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes, err := s.loadScopeLocked(scope)
	if err != nil {
		return nil, err
	}
	n, ok := nodes[id]
	if !ok {
		return nil, fmt.Errorf("node %q not found in scope %q", id, scope)
	}
	cp := cloneNode(n)
	return &cp, nil
}

// Delete removes one node.
func (s *Store) Delete(scope, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes, err := s.loadScopeLocked(scope)
	if err != nil {
		return err
	}
	if _, ok := nodes[id]; !ok {
		return fmt.Errorf("node %q not found in scope %q", id, scope)
	}
	delete(nodes, id)
	return s.saveScopeLocked(scope)
}

// List returns nodes in a scope visible to view, newest-updated first,
// optionally filtered by type. limit <= 0 means no limit.
func (s *Store) List(scope, nodeType string, limit int, view View) ([]Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes, err := s.loadScopeLocked(scope)
	if err != nil {
		return nil, err
	}
	t := view.at()
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if nodeType != "" && n.Type != nodeType {
			continue
		}
		if !view.visible(n, t) {
			continue
		}
		out = append(out, cloneNode(n))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// AddLink creates (or re-weights) a typed edge from sourceID to targetID
// inside scope. The source must exist; the target may dangle (cross-scope or
// not-yet-written nodes are deliberately allowed).
func (s *Store) AddLink(scope, sourceID, targetID, relation string, weight float64) error {
	if !validRelations[relation] {
		return fmt.Errorf("invalid relation %q (want applies_to|depends_on|part_of|related_to|supersedes|contradicts)", relation)
	}
	if weight <= 0 {
		weight = 0.5
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes, err := s.loadScopeLocked(scope)
	if err != nil {
		return err
	}
	src, ok := nodes[sourceID]
	if !ok {
		return fmt.Errorf("source node %q not found in scope %q", sourceID, scope)
	}
	for i := range src.Links {
		if src.Links[i].Target == targetID && src.Links[i].Relation == relation {
			src.Links[i].Weight = weight
			src.Updated = time.Now().UTC()
			return s.saveScopeLocked(scope)
		}
	}
	src.Links = append(src.Links, Link{Target: targetID, Relation: relation, Weight: weight})
	src.Updated = time.Now().UTC()
	return s.saveScopeLocked(scope)
}

// Query ranks nodes across the given scopes visible to view against a
// free-text query using BM25 over title+content+tags, with mild priority and
// recency boosts. limit <= 0 defaults to 10.
func (s *Store) Query(query string, scopes []string, nodeType string, limit int, view View) ([]Scored, error) {
	if limit <= 0 {
		limit = 10
	}
	t := view.at()
	s.mu.Lock()
	pool := make([]*Node, 0, 64)
	for _, scope := range scopes {
		nodes, err := s.loadScopeLocked(scope)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		for _, n := range nodes {
			if nodeType != "" && n.Type != nodeType {
				continue
			}
			if !view.visible(n, t) {
				continue
			}
			cp := cloneNode(n)
			pool = append(pool, &cp)
		}
	}
	s.mu.Unlock()
	return rankNodes(query, pool, limit, t), nil
}
