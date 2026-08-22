package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadChatTreeRoundTrip(t *testing.T) {
	s := newTestStore(t)
	id := "chat-abc-123"
	want := []byte(`{"schema_version":1,"tree":{"nodes":{}}}`)

	if err := s.SaveChatTree(id, want); err != nil {
		t.Fatalf("SaveChatTree: %v", err)
	}

	// File exists at the expected path.
	path := filepath.Join(s.dir, id+".chat.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s on disk: %v", path, err)
	}

	got, err := s.LoadChatTree(id)
	if err != nil {
		t.Fatalf("LoadChatTree: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("LoadChatTree returned %q, want %q", got, want)
	}
}

func TestSaveChatTreeAtomicNoTmpLeak(t *testing.T) {
	// After a successful save there must be no .tmp left behind in the
	// sessions directory.
	s := newTestStore(t)
	if err := s.SaveChatTree("chat-1", []byte("{}")); err != nil {
		t.Fatalf("SaveChatTree: %v", err)
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("stray tmp file after save: %s", e.Name())
		}
	}
}

func TestLoadChatTreeMissingIsNotExist(t *testing.T) {
	s := newTestStore(t)
	_, err := s.LoadChatTree("chat-never-saved")
	if err == nil {
		t.Fatal("expected error for missing chat tree")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist via errors.Is, got %v", err)
	}
}

func TestChatTreePathTraversalRejected(t *testing.T) {
	s := newTestStore(t)
	cases := []string{
		"../escape",
		"sub/dir",
		`back\slash`,
		"",
	}
	for _, id := range cases {
		if err := s.SaveChatTree(id, []byte("{}")); err == nil {
			t.Errorf("SaveChatTree(%q) accepted invalid id", id)
		}
		if _, err := s.LoadChatTree(id); err == nil {
			t.Errorf("LoadChatTree(%q) accepted invalid id", id)
		}
	}
}

func TestLoadChatTreeRejectsOversize(t *testing.T) {
	// A blob larger than maxChatTreeSize must fail loud at Stat time —
	// before any memory is allocated to read it — so a corrupt or
	// runaway-producer .chat.json file can't OOM the serve process.
	s := newTestStore(t)
	id := "chat-too-big"

	// Write a sparse file at maxChatTreeSize+1 bytes using truncate.
	// Truncate-to-size is fine — it produces a hole on most filesystems,
	// so the test doesn't actually need to write 64MiB of data.
	path := filepath.Join(s.dir, id+".chat.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxChatTreeSize + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	_, err = s.LoadChatTree(id)
	if err == nil {
		t.Fatal("expected oversize file to be rejected")
	}
	if !strings.Contains(err.Error(), "cap") {
		t.Errorf("error should mention the cap: %v", err)
	}
}

func TestSaveChatTreeOverwrites(t *testing.T) {
	// Second SaveChatTree on the same id replaces the prior content.
	s := newTestStore(t)
	id := "chat-overwrite"
	if err := s.SaveChatTree(id, []byte(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveChatTree(id, []byte(`{"v":2}`)); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadChatTree(id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"v":2}` {
		t.Fatalf("got %q, want second write to win", got)
	}
}
