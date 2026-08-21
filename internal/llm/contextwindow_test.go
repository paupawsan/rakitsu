package llm

import "testing"

func TestLookupContextWindow_KnownModels(t *testing.T) {
	cases := map[string]int{
		"gpt-4o-mini":       128000,
		"gpt-4o":            128000,
		"claude-sonnet-4-5": 200000,
		"claude-opus-4-6":   200000,
		"claude-haiku-4-5":  200000,
		"gemini-2.5-pro":    1000000,
		"gemini-2.5-flash":  1000000,
	}
	for model, want := range cases {
		got, known := LookupContextWindow(model)
		if !known {
			t.Errorf("LookupContextWindow(%q) known=false, want true", model)
		}
		if got != want {
			t.Errorf("LookupContextWindow(%q) = %d, want %d", model, got, want)
		}
	}
}

func TestLookupContextWindow_UnknownModel(t *testing.T) {
	got, known := LookupContextWindow("my-custom-litellm-route")
	if known {
		t.Errorf("known = true, want false for an unmapped model")
	}
	if got != 0 {
		t.Errorf("got = %d, want 0 for unknown model", got)
	}
}

func TestLookupContextWindow_PrefixMatch(t *testing.T) {
	// A dated/suffixed variant of a known family should still resolve via
	// prefix match, the same way tiktoken-go handles model aliases.
	got, known := LookupContextWindow("gpt-4o-2024-08-06")
	if !known || got != 128000 {
		t.Errorf("LookupContextWindow(gpt-4o-2024-08-06) = (%d, %v), want (128000, true) via prefix match", got, known)
	}
}
