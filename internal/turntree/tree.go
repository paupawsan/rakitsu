// Package turntree provides a generic tree of conversation turns where the
// "active conversation" is the unique root-to-leaf path selected by the user.
//
// A Tree is the source of truth for branching chat history. Each Node holds
// the user message that started the turn plus the response Entries produced
// for that turn. Branches diverge when an older turn is edited or
// regenerated: the new turn becomes a sibling of the original, and
// ActiveChild controls which sibling lies on the visible path.
//
// The package is generic over the entry payload (E) so it can back the web
// chat (transcriptEntry) and the TUI chat (ContentBlock) without
// duplication. The package is pure: no I/O, no goroutines. Thread-safety is
// the caller's responsibility — the existing ChatSession.mu and the
// single-threaded bubbletea TUI both cover it naturally.
package turntree

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Status is the terminal state of a turn's generation.
type Status string

const (
	StatusGenerating  Status = "generating"
	StatusComplete    Status = "complete"
	StatusInterrupted Status = "interrupted"
	StatusFailed      Status = "failed"
)

// RetryContext is the reason a turn was created as a sibling instead of a
// fresh child. Producers populate this when an agent-driven self-correction
// (rollback, salvage, auto-regenerate) forks the conversation so the model
// can see the failure mode of the prior attempt; consumers that rebuild
// model-visible history (e.g. ChatSession.historyFromTranscript) surface it
// as a brief system directive on the new turn.
//
// Left nil for ordinary turns and for user-driven branches (edit/regenerate),
// where the new user text already carries the correction.
type RetryContext struct {
	Source      string `json:"source"`                    // "rollback" | "salvage" | "auto-regenerate" | ...
	PriorTurnID string `json:"prior_turn_id,omitempty"`   // sibling whose failure triggered this branch
	Reason      string `json:"reason"`                    // 1-line, model-readable; ≤200 chars by convention
}

// Node is a single conversation turn: the user message that started it and
// the response blocks produced in reply.
type Node[E any] struct {
	ID       string        `json:"id"`
	ParentID string        `json:"parent_id,omitempty"`
	UserText string        `json:"user_text"`
	Entries  []E           `json:"entries,omitempty"`
	Children []string      `json:"children,omitempty"`
	Status   Status        `json:"status"`
	Created  time.Time     `json:"created"`
	RetryCtx *RetryContext `json:"retry_ctx,omitempty"`
}

// Tree is a forest of turns. The active conversation is the unique chain
// from the active root through ActiveChild at each visited node.
type Tree[E any] struct {
	Nodes       map[string]*Node[E] `json:"nodes"`
	Roots       []string            `json:"roots,omitempty"`
	ActiveChild map[string]string   `json:"active_child,omitempty"`
}

// Sentinel errors for tree operations.
var (
	ErrEmptyID       = errors.New("turntree: empty id")
	ErrDuplicateID   = errors.New("turntree: duplicate id")
	ErrUnknownNode   = errors.New("turntree: unknown node")
	ErrUnknownParent = errors.New("turntree: unknown parent")
	ErrSiblingCount  = errors.New("turntree: no sibling to switch to")
)

// New returns an empty Tree.
func New[E any]() *Tree[E] {
	return &Tree[E]{
		Nodes:       make(map[string]*Node[E]),
		ActiveChild: make(map[string]string),
	}
}

// AppendTurn creates a new turn as a child of parentID (or as a root when
// parentID is empty) and makes it the active sibling at that level. The
// new node starts in Generating status with no Entries.
//
// Returns ErrEmptyID, ErrDuplicateID, or ErrUnknownParent on invalid input.
func (t *Tree[E]) AppendTurn(id, userText, parentID string) (*Node[E], error) {
	if err := t.checkNewID(id); err != nil {
		return nil, err
	}
	if parentID != "" {
		parent, ok := t.Nodes[parentID]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownParent, parentID)
		}
		parent.Children = append(parent.Children, id)
	} else {
		t.Roots = append(t.Roots, id)
	}
	node := &Node[E]{
		ID:       id,
		ParentID: parentID,
		UserText: userText,
		Status:   StatusGenerating,
		Created:  time.Now().UTC(),
	}
	t.Nodes[id] = node
	t.ActiveChild[parentID] = id
	return node, nil
}

// AddSibling creates a new turn at the same level as targetID — same
// parent — and makes it the active sibling. Used by both edit (newUserText
// differs) and regenerate (newUserText equals the target's UserText).
func (t *Tree[E]) AddSibling(id, targetID, newUserText string) (*Node[E], error) {
	target, ok := t.Nodes[targetID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownNode, targetID)
	}
	return t.AppendTurn(id, newUserText, target.ParentID)
}

// SwitchToSibling shifts the active sibling at targetID's level by dir
// (typically ±1). Cycling wraps Python-style so dir=-1 from index 0 with
// count=3 lands on index 2. Returns the id of the new active sibling.
//
// If targetID has no siblings, returns (targetID, ErrSiblingCount).
func (t *Tree[E]) SwitchToSibling(targetID string, dir int) (string, error) {
	target, ok := t.Nodes[targetID]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownNode, targetID)
	}
	siblings := t.siblings(target.ParentID)
	n := len(siblings)
	if n <= 1 {
		return targetID, ErrSiblingCount
	}
	idx := indexOf(siblings, targetID)
	if idx < 0 {
		// Defensive: should never happen if tree invariants hold.
		return "", fmt.Errorf("%w: %s", ErrUnknownNode, targetID)
	}
	newIdx := ((idx+dir)%n + n) % n
	newID := siblings[newIdx]
	t.ActiveChild[target.ParentID] = newID
	return newID, nil
}

// SetActivePath walks from leafTurnID up to its root, setting ActiveChild
// at each parent so leafTurnID sits on the active path. Used by the Tree
// Viewer's "Make active" affordance.
func (t *Tree[E]) SetActivePath(leafTurnID string) error {
	cur, ok := t.Nodes[leafTurnID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownNode, leafTurnID)
	}
	for cur != nil {
		t.ActiveChild[cur.ParentID] = cur.ID
		if cur.ParentID == "" {
			break
		}
		parent, ok := t.Nodes[cur.ParentID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownParent, cur.ParentID)
		}
		cur = parent
	}
	return nil
}

// ActivePath returns the chain of nodes from the active root to the active
// leaf. An empty tree yields nil. When ActiveChild for a visited node is
// unset, the first child (if any) is taken so the path keeps descending —
// this keeps freshly-loaded trees usable when an older save omitted entries.
func (t *Tree[E]) ActivePath() []*Node[E] {
	if len(t.Nodes) == 0 {
		return nil
	}
	rootID := t.ActiveChild[""]
	if rootID == "" && len(t.Roots) > 0 {
		rootID = t.Roots[0]
	}
	if rootID == "" {
		return nil
	}
	out := make([]*Node[E], 0, 4)
	seen := make(map[string]struct{}) // cycle guard against corrupt data
	curID := rootID
	for curID != "" {
		if _, dup := seen[curID]; dup {
			break
		}
		seen[curID] = struct{}{}
		node, ok := t.Nodes[curID]
		if !ok {
			break
		}
		out = append(out, node)
		nextID := t.ActiveChild[node.ID]
		if nextID == "" && len(node.Children) > 0 {
			nextID = node.Children[0]
		}
		curID = nextID
	}
	return out
}

// ActiveLeaf returns the last node on the active path, or nil for an empty
// tree.
func (t *Tree[E]) ActiveLeaf() *Node[E] {
	path := t.ActivePath()
	if len(path) == 0 {
		return nil
	}
	return path[len(path)-1]
}

// SiblingIndex returns (index, count) for the position of id among its
// siblings. Returns (-1, 0) when id is unknown.
func (t *Tree[E]) SiblingIndex(id string) (int, int) {
	node, ok := t.Nodes[id]
	if !ok {
		return -1, 0
	}
	sibs := t.siblings(node.ParentID)
	return indexOf(sibs, id), len(sibs)
}

// Get returns the node with the given id.
func (t *Tree[E]) Get(id string) (*Node[E], bool) {
	n, ok := t.Nodes[id]
	return n, ok
}

// AppendEntry adds one response block to the turn's Entries.
func (t *Tree[E]) AppendEntry(turnID string, entry E) error {
	node, ok := t.Nodes[turnID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownNode, turnID)
	}
	node.Entries = append(node.Entries, entry)
	return nil
}

// Finalize sets the terminal status of a turn.
func (t *Tree[E]) Finalize(turnID string, status Status) error {
	node, ok := t.Nodes[turnID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownNode, turnID)
	}
	node.Status = status
	return nil
}

// Hydrate ensures Nodes and ActiveChild are non-nil. Idempotent. Useful
// after json.Unmarshal into a Tree literal, or when constructing a Tree
// programmatically without the New constructor.
func (t *Tree[E]) Hydrate() {
	if t.Nodes == nil {
		t.Nodes = make(map[string]*Node[E])
	}
	if t.ActiveChild == nil {
		t.ActiveChild = make(map[string]string)
	}
}

// LoadFromJSON unmarshals JSON bytes into a new Tree[E] and hydrates its
// maps. Convenience for the persistence layer.
func LoadFromJSON[E any](data []byte) (*Tree[E], error) {
	t := New[E]()
	if err := json.Unmarshal(data, t); err != nil {
		return nil, err
	}
	t.Hydrate()
	return t, nil
}

// internal helpers

func (t *Tree[E]) checkNewID(id string) error {
	if id == "" {
		return ErrEmptyID
	}
	if _, exists := t.Nodes[id]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateID, id)
	}
	return nil
}

func (t *Tree[E]) siblings(parentID string) []string {
	if parentID == "" {
		return t.Roots
	}
	parent, ok := t.Nodes[parentID]
	if !ok {
		return nil
	}
	return parent.Children
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}
