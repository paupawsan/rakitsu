package llm

import "testing"

func TestNormalizeArgKeys_TrimsWhitespace(t *testing.T) {
	args := map[string]interface{}{
		" args":   "hello",
		"normal":  42,
		"  both ": true,
	}
	got := NormalizeArgKeys(args)
	if _, ok := got["args"]; !ok {
		t.Errorf("expected trimmed key 'args', got keys %v", keys(got))
	}
	if _, ok := got["normal"]; !ok {
		t.Errorf("expected key 'normal' to survive, got keys %v", keys(got))
	}
	if _, ok := got["both"]; !ok {
		t.Errorf("expected trimmed key 'both', got keys %v", keys(got))
	}
	if len(got) != 3 {
		t.Errorf("expected 3 keys, got %d: %v", len(got), keys(got))
	}
}

func TestNormalizeArgKeys_NoOpWhenClean(t *testing.T) {
	args := map[string]interface{}{"a": 1, "b": 2}
	got := NormalizeArgKeys(args)
	if len(got) != 2 || got["a"] != 1 || got["b"] != 2 {
		t.Errorf("clean keys should pass through unchanged, got %v", got)
	}
}

func TestNormalizeArgKeys_NilSafe(t *testing.T) {
	got := NormalizeArgKeys(nil)
	if got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
}

func TestNormalizeArgKeys_CollisionDeterministic(t *testing.T) {
	// Regression: mutating the map while ranging over it made a collision
	// between a whitespace-padded key and its trimmed form undefined —
	// which value survived depended on Go's randomized map iteration order.
	// The fix builds a fresh map, so the result must be the same every run.
	for i := 0; i < 20; i++ {
		args := map[string]interface{}{" x": 1, "x": 2}
		got := NormalizeArgKeys(args)
		if len(got) != 1 {
			t.Fatalf("expected exactly 1 key after collision, got %d: %v", len(got), keys(got))
		}
		if _, ok := got["x"]; !ok {
			t.Fatalf("expected key 'x' to survive, got %v", keys(got))
		}
	}
}

func keys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
