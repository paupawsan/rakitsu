// Package acp implements an ACP (Agent Client Protocol) server.
// ACP is JSON-RPC 2.0 over stdio, adopted by JetBrains, Zed, Cursor, and others
// as a standard protocol for invoking coding agents from IDEs.
package acp

import (
	"context"
	"sync"
)

// sessionStatus represents the lifecycle state of a single ACP run.
type sessionStatus string

const (
	statusRunning   sessionStatus = "running"
	statusCompleted sessionStatus = "completed"
	statusError     sessionStatus = "error"
	statusCancelled sessionStatus = "cancelled"
)

// Session tracks one active agent/run request.
type Session struct {
	ID     string
	cancel context.CancelFunc

	mu     sync.Mutex
	status sessionStatus
	result string
	errMsg string
}

func newSession(id string, cancel context.CancelFunc) *Session {
	return &Session{
		ID:     id,
		cancel: cancel,
		status: statusRunning,
	}
}

// complete records a successful run — unless the session already reached a
// terminal state (cancelled or, symmetrically with fail(), error). The
// current call sites (all in handleRun) never actually call both complete()
// and fail() for the same session, so the error case isn't reachable today —
// but checking only cancelled here while cancelled() itself guards both
// completed and error was an asymmetry that any future call site could fall
// into. Mirrors fail()'s guard below.
func (s *Session) complete(result string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == statusCancelled || s.status == statusError {
		return
	}
	s.status = statusCompleted
	s.result = result
}

// fail records a run failure — unless the session already reached a terminal
// state (cancelled or, symmetrically with complete(), completed). A
// cancelled run's own goroutine typically observes context cancellation and
// calls fail() with a "context canceled"-flavored error shortly after
// handleCancel already recorded the cancellation; without the cancelled
// guard that call would silently clobber the terminal "cancelled" status
// back to "error". The completed guard closes the mirror image for the same
// reason cancelled() itself already checks both terminal states.
func (s *Session) fail(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == statusCancelled || s.status == statusCompleted {
		return
	}
	s.status = statusError
	s.errMsg = msg
}

// cancelled records a run cancellation — unless the session already reached
// a terminal state. Mirrors the guard on complete()/fail(): the run
// goroutine's own completion path can land between handleCancel's
// sess.cancel() and sess.cancelled() calls (the session isn't removed from
// the registry until the goroutine's deferred delete, after its
// "agent/complete" notification is already sent), and without this guard a
// late-arriving agent/cancel would silently overwrite an already-reported
// "completed"/"error" outcome back to "cancelled".
func (s *Session) cancelled() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == statusCompleted || s.status == statusError {
		return
	}
	s.status = statusCancelled
}

func (s *Session) snapshot() (sessionStatus, string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, s.result, s.errMsg
}

// sessionMap is a thread-safe registry of active sessions.
type sessionMap struct {
	mu   sync.Mutex
	data map[string]*Session
}

func newSessionMap() *sessionMap {
	return &sessionMap{data: make(map[string]*Session)}
}

func (m *sessionMap) add(s *Session) {
	m.mu.Lock()
	m.data[s.ID] = s
	m.mu.Unlock()
}

func (m *sessionMap) get(id string) (*Session, bool) {
	m.mu.Lock()
	s, ok := m.data[id]
	m.mu.Unlock()
	return s, ok
}

func (m *sessionMap) delete(id string) {
	m.mu.Lock()
	delete(m.data, id)
	m.mu.Unlock()
}
