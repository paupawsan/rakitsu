package server

import (
	"fmt"
	"testing"
	"time"
)

func newTestMailboxes(now *time.Time) *mailboxStore {
	s := newMailboxStore()
	s.now = func() time.Time { return *now }
	return s
}

func TestMailboxDeliverRequiresProvision(t *testing.T) {
	now := time.Now()
	s := newTestMailboxes(&now)
	if s.deliver("ghost", MailboxMsg{Text: "x"}) {
		t.Fatal("deliver to unprovisioned id must fail")
	}
	s.provision("cc-1")
	if !s.deliver("cc-1", MailboxMsg{Text: "hello"}) {
		t.Fatal("deliver after provision must succeed")
	}
	msgs, ok := s.drain("cc-1")
	if !ok || len(msgs) != 1 || msgs[0].Text != "hello" {
		t.Fatalf("drain = %v %v", msgs, ok)
	}
	// drain-on-read clears
	msgs, ok = s.drain("cc-1")
	if !ok || len(msgs) != 0 {
		t.Fatalf("second drain must be empty, got %v", msgs)
	}
	if _, ok := s.drain("ghost"); ok {
		t.Fatal("drain of unknown id must report not-found")
	}
}

func TestMailboxOverflowDropsOldest(t *testing.T) {
	now := time.Now()
	s := newTestMailboxes(&now)
	s.provision("cc-1")
	for i := 0; i < mailboxMaxMsgs+3; i++ {
		s.deliver("cc-1", MailboxMsg{Text: string(rune('a' + i%26)), FromSessionID: "r"})
	}
	msgs, _ := s.drain("cc-1")
	if len(msgs) != mailboxMaxMsgs {
		t.Fatalf("len = %d, want %d", len(msgs), mailboxMaxMsgs)
	}
	// oldest 3 dropped: first surviving message is the 4th delivered ('d')
	if msgs[0].Text != "d" {
		t.Fatalf("oldest not dropped, first = %q", msgs[0].Text)
	}
}

func TestMailboxTTLExpiry(t *testing.T) {
	now := time.Now()
	s := newTestMailboxes(&now)
	s.provision("cc-1")
	s.deliver("cc-1", MailboxMsg{Text: "x"})
	now = now.Add(mailboxTTL + time.Second)
	if _, ok := s.drain("cc-1"); ok {
		t.Fatal("expired mailbox must be swept, drain reports not-found")
	}
}

func TestMailboxGlobalCapEvictsLRU(t *testing.T) {
	now := time.Now()
	s := newTestMailboxes(&now)
	s.provision("oldest")
	for i := 0; i < mailboxMaxBoxes; i++ {
		now = now.Add(time.Second)
		s.provision(fmt.Sprintf("box-%d", i))
	}
	if _, ok := s.drain("oldest"); ok {
		t.Fatal("LRU mailbox must be evicted at global cap")
	}
	if _, ok := s.drain(fmt.Sprintf("box-%d", mailboxMaxBoxes-1)); !ok {
		t.Fatal("newest mailbox must survive")
	}
}
