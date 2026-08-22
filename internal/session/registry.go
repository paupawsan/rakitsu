package session

import (
	"sort"
	"sync"
)

// Registry is a read-mostly index of live Session instances. Phase 1 is
// additive: concrete owners (AgentRunner, ChatManager) call Register and
// Unregister around their existing lifecycle edges. No behavior on the owned
// sessions changes — the registry is purely observational.
//
// Phase 4 will collapse the owners into the registry itself; until then the
// registry is a facade.
type Registry struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{sessions: make(map[string]Session)}
}

// Register adds s to the registry. If a session with the same ID is already
// registered it is replaced — the caller is expected to have already cleaned
// up the prior instance.
func (r *Registry) Register(s Session) {
	if s == nil {
		return
	}
	id := s.ID()
	if id == "" {
		return
	}
	r.mu.Lock()
	r.sessions[id] = s
	r.mu.Unlock()
}

// Unregister removes the session with the given id. Returns true if a session
// was removed.
func (r *Registry) Unregister(id string) bool {
	r.mu.Lock()
	_, ok := r.sessions[id]
	delete(r.sessions, id)
	r.mu.Unlock()
	return ok
}

// Get returns the session for id, or ErrNotFound.
func (r *Registry) Get(id string) (Session, error) {
	r.mu.RLock()
	s, ok := r.sessions[id]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

// List returns metadata snapshots for all registered sessions matching the
// filter, sorted by StartedAt ascending (oldest first so the UI can stack
// live sessions at the top deterministically).
func (r *Registry) List(f Filter) []SessionMeta {
	r.mu.RLock()
	snapshots := make([]SessionMeta, 0, len(r.sessions))
	for _, s := range r.sessions {
		m := s.Meta()
		if !f.Match(m) {
			continue
		}
		snapshots = append(snapshots, m)
	}
	r.mu.RUnlock()

	sort.SliceStable(snapshots, func(i, j int) bool {
		return snapshots[i].StartedAt < snapshots[j].StartedAt
	})
	return snapshots
}

// Len returns the number of registered sessions (primarily for tests).
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sessions)
}
