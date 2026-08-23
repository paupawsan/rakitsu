package agent

import "testing"

// TestClaimsStorage pins the storage-claim detector: it must catch
// fabricated memory-write confirmations while staying quiet on
// instructional, idiomatic, and file-side-effect phrasing.
func TestClaimsStorage(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		// --- fabrications (the bug) ---
		{"got_it_stored", "Got it—I've stored that your release versioning is v0.2.0.", true},
		{"ive_stored_that", "I've stored that for you.", true},
		{"have_recorded_your", "I have recorded your preference.", true},
		{"weve_saved_it", "We've saved it to memory.", true},
		{"persisted_the", "I persisted the fact you mentioned.", true},
		{"noted_that_down", "I've noted that down.", true},
		{"saved_as_quoted_id", "Done. Saved as `user-pref-tz`.", true},
		{"persisted_as_bare_id", "Persisted as session-recordings-fact.", true},
		{"saved_it_as_quoted", "I saved it as `note-1`.", true},
		{"ive_retired_that", "I've retired that old entry.", true},
		{"ive_retired_the_rule", "I've retired the rule that was outdated.", true},
		{"got_it_retired", "Got it—I've retired the old fact.", true},
		{"retired_bare_id", "Retired old-deploy-checklist.", true},
		{"superseded_quoted_id", "I've superseded `deploy-v1` with a new entry.", true},

		// --- not fabrications (must stay quiet) ---
		{"interrogative", "How do I save a file?", false},
		{"instructional_to_save", "To save a file, press Ctrl+S.", false},
		{"instructional_you_can", "You can persist this with the memory_add tool.", false},
		{"idiom_saved_you_time", "I've saved you a lot of time on this.", false},
		{"file_side_effect", "I saved the file to disk at /tmp/out.txt.", false},
		{"saved_as_prose", "I saved the draft as a backup copy.", false},
		{"empty", "", false},
		{"unrelated", "The capital of France is Paris.", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := claimsStorage(c.text); got != c.want {
				t.Errorf("claimsStorage(%q) = %v, want %v", c.text, got, c.want)
			}
		})
	}
}

// TestTurnGuard_Check exercises the guard end-to-end over its two enabled
// checks, including the ground-truth cross-checks that make a correct turn
// impossible to flag.
func TestTurnGuard_Check(t *testing.T) {
	cases := []struct {
		name       string
		cfg        TurnGuardConfig
		answer     string
		sig        turnSignals
		wantMarker string
	}{
		// no-delegation
		{
			name:       "no_delegation_fires",
			cfg:        TurnGuardConfig{ForceDelegation: true},
			answer:     "Sure, here's the answer from my own knowledge.",
			sig:        turnSignals{invokeConfigCalled: false},
			wantMarker: markerNoDelegation,
		},
		{
			name:       "delegation_happened_ok",
			cfg:        TurnGuardConfig{ForceDelegation: true},
			answer:     "Here is the relayed answer.",
			sig:        turnSignals{invokeConfigCalled: true},
			wantMarker: "",
		},
		{
			name:       "force_delegation_off_ignores_signal",
			cfg:        TurnGuardConfig{ForceDelegation: false},
			answer:     "answer",
			sig:        turnSignals{invokeConfigCalled: false},
			wantMarker: "",
		},
		// fabricated-store
		{
			name:       "fabricated_store_fires",
			cfg:        TurnGuardConfig{CheckStorageClaims: true},
			answer:     "Got it—I've stored that for you.",
			sig:        turnSignals{memoryWriteCount: 0},
			wantMarker: markerUnverified,
		},
		{
			name:       "real_write_not_flagged",
			cfg:        TurnGuardConfig{CheckStorageClaims: true},
			answer:     "I've stored that for you.",
			sig:        turnSignals{memoryWriteCount: 1},
			wantMarker: "",
		},
		{
			name:       "storage_check_off_ignores_claim",
			cfg:        TurnGuardConfig{CheckStorageClaims: false},
			answer:     "I've stored that for you.",
			sig:        turnSignals{memoryWriteCount: 0},
			wantMarker: "",
		},
		{
			name:       "instructional_not_flagged",
			cfg:        TurnGuardConfig{CheckStorageClaims: true},
			answer:     "To save a file, press Ctrl+S.",
			sig:        turnSignals{memoryWriteCount: 0},
			wantMarker: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewTurnGuard(c.cfg)
			marker, reason := g.Check(c.answer, c.sig)
			if marker != c.wantMarker {
				t.Errorf("Check() marker = %q, want %q", marker, c.wantMarker)
			}
			if marker != "" && reason == "" {
				t.Errorf("Check() returned marker %q with empty reason", marker)
			}
		})
	}
}

// TestTurnGuard_NilIsNoop pins that a nil guard accepts everything — the
// zero-overhead default for agents that never call SetTurnGuard.
func TestTurnGuard_NilIsNoop(t *testing.T) {
	var g *TurnGuard
	if marker, _ := g.Check("I've stored that.", turnSignals{}); marker != "" {
		t.Errorf("nil guard should accept, got marker %q", marker)
	}
}

func TestForceDelegationAcceptsSpawn(t *testing.T) {
	g := NewTurnGuard(TurnGuardConfig{ForceDelegation: true})
	if marker, _ := g.Check("done", turnSignals{spawnCalled: true}); marker != "" {
		t.Errorf("spawnCalled turn rejected with %q, want accept", marker)
	}
	if marker, _ := g.Check("done", turnSignals{}); marker != markerNoDelegation {
		t.Errorf("no-signal turn = %q, want %q", marker, markerNoDelegation)
	}
}
