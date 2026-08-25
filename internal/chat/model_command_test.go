package chat

import "testing"

func TestParseModelSpec(t *testing.T) {
	cases := []struct {
		name         string
		in           string
		wantModel    string
		wantProvider string
	}{
		{"empty", "", "", ""},
		{"whitespace only", "   ", "", ""},
		{"model only", "gemini-3.1-flash-lite", "gemini-3.1-flash-lite", ""},
		{"model + provider", "gemini-3.1-flash-lite@litellm-gemini-flash", "gemini-3.1-flash-lite", "litellm-gemini-flash"},
		{"whitespace around at", "gemini-3.1-flash-lite @ litellm-gemini-flash", "gemini-3.1-flash-lite", "litellm-gemini-flash"},
		{"model name containing slash", "vendor/model@providerX", "vendor/model", "providerX"},
		// Model names can legitimately contain colons (e.g. ollama tags like "llama3:8b").
		// We split on the LAST '@' so colons don't interfere.
		{"colon in model", "llama3:8b", "llama3:8b", ""},
		{"colon in model + provider", "llama3:8b@ollama-local", "llama3:8b", "ollama-local"},
		// Edge: '@' inside model name — last-@ rule keeps the rightmost one as the split.
		// This is the documented behavior; users with literal '@' in model names must
		// pass an explicit @provider to disambiguate.
		{"multiple ats", "a@b@c", "a@b", "c"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotModel, gotProvider := ParseModelSpec(c.in)
			if gotModel != c.wantModel || gotProvider != c.wantProvider {
				t.Errorf("ParseModelSpec(%q) = (%q, %q); want (%q, %q)",
					c.in, gotModel, gotProvider, c.wantModel, c.wantProvider)
			}
		})
	}
}
