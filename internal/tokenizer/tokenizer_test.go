package tokenizer

import (
	"strings"
	"testing"
)

func TestTokenize_KnownModel(t *testing.T) {
	resp, err := Tokenize("Hello world", "gpt-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total == 0 {
		t.Fatal("expected non-zero token count")
	}
	if resp.Encoding == "" {
		t.Fatal("expected encoding name")
	}
	if strings.Contains(resp.Encoding, "approx") {
		t.Fatalf("expected exact encoding for gpt-4, got %q", resp.Encoding)
	}
}

func TestTokenize_UnknownModel(t *testing.T) {
	resp, err := Tokenize("Hello world", "claude-3-opus")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total == 0 {
		t.Fatal("expected non-zero token count")
	}
	if !strings.Contains(resp.Encoding, "approx") {
		t.Fatalf("expected approx encoding for unknown model, got %q", resp.Encoding)
	}
}

func TestTokenize_PrefixMatchedModelReportsCorrectEncoding(t *testing.T) {
	// Regression: "gpt-4o-mini" only resolves via EncodingForModel's prefix
	// fallback ("gpt-4o-" -> o200k_base), not an exact MODEL_TO_ENCODING
	// match. A second exact-match-only lookup used just for the label used
	// to silently mislabel this as "cl100k_base" with no approx flag.
	resp, err := Tokenize("Hello world", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(resp.Encoding, "approx") {
		t.Fatalf("expected exact (prefix-resolved) encoding for gpt-4o-mini, got %q", resp.Encoding)
	}
	if resp.Encoding != "o200k_base" {
		t.Fatalf("expected label to reflect the actually-resolved encoding, got %q", resp.Encoding)
	}
}

func TestTokenize_EmptyText(t *testing.T) {
	resp, err := Tokenize("", "gpt-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 0 {
		t.Fatalf("expected 0 tokens for empty text, got %d", resp.Total)
	}
}

func TestTokenize_RoundTrip(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog."
	resp, err := Tokenize(text, "gpt-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var rebuilt strings.Builder
	for _, tok := range resp.Tokens {
		rebuilt.WriteString(tok.Text)
	}
	if rebuilt.String() != text {
		t.Fatalf("round-trip mismatch:\n  got:  %q\n  want: %q", rebuilt.String(), text)
	}
}
