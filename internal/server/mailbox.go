package server

import (
	"sync"
	"time"
)

// External-sender mailboxes (spec §3): the hub stores messages addressed to
// non-live sender IDs so foreign agents (curl, Claude Code, scripts) get a
// pollable reply path without running rakitsu. In-memory only — a serve
// restart clears all boxes. IDs are claim-based; the shared bearer token is
// the only gate (documented localhost/Tailscale trust assumption).
const (
	mailboxMaxMsgs  = 32
	mailboxTTL      = 15 * time.Minute
	mailboxMaxBoxes = 256
)

// MailboxMsg is one stored message, returned verbatim by the inbox endpoint.
type MailboxMsg struct {
	Text          string    `json:"text"`
	FromSessionID string    `json:"from_session_id,omitempty"`
	FromName      string    `json:"from_name,omitempty"`
	ReceivedAt    time.Time `json:"received_at"`
}

type mailbox struct {
	msgs      []MailboxMsg
	lastTouch time.Time
}

// mailboxStore is guarded by its own mutex, never SSEServer.mu — mailbox
// traffic must not serialize hub register/poll/ingest.
type mailboxStore struct {
	mu    sync.Mutex
	now   func() time.Time // injectable for tests
	boxes map[string]*mailbox
}

func newMailboxStore() *mailboxStore {
	return &mailboxStore{now: time.Now, boxes: make(map[string]*mailbox)}
}

// sweep drops expired boxes and, at the global cap, the least-recently
// touched one. Called under mu from every public method.
func (s *mailboxStore) sweep() {
	now := s.now()
	for id, b := range s.boxes {
		if now.Sub(b.lastTouch) > mailboxTTL {
			delete(s.boxes, id)
		}
	}
	for len(s.boxes) > mailboxMaxBoxes {
		lruID, lruT := "", now
		for id, b := range s.boxes {
			if !b.lastTouch.After(lruT) {
				lruID, lruT = id, b.lastTouch
			}
		}
		if lruID == "" {
			break // guards a theoretical infinite loop with non-monotonic clocks
		}
		delete(s.boxes, lruID)
	}
}

// provision creates the id's mailbox if absent and refreshes its TTL.
func (s *mailboxStore) provision(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.boxes[id]; ok {
		b.lastTouch = s.now()
	} else {
		s.boxes[id] = &mailbox{lastTouch: s.now()}
	}
	s.sweep()
}

// deliver appends to an existing mailbox (false when none — the caller then
// falls through to not-found). Overflow drops the oldest message.
func (s *mailboxStore) deliver(id string, m MailboxMsg) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep()
	b, ok := s.boxes[id]
	if !ok {
		return false
	}
	m.ReceivedAt = s.now()
	b.msgs = append(b.msgs, m)
	if len(b.msgs) > mailboxMaxMsgs {
		b.msgs = b.msgs[len(b.msgs)-mailboxMaxMsgs:]
	}
	b.lastTouch = s.now()
	return true
}

// drain returns and clears the box's messages; ok=false when no box exists.
func (s *mailboxStore) drain(id string) ([]MailboxMsg, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep()
	b, ok := s.boxes[id]
	if !ok {
		return nil, false
	}
	out := b.msgs
	b.msgs = nil
	b.lastTouch = s.now()
	if out == nil {
		out = []MailboxMsg{}
	}
	return out, true
}
