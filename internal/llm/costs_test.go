package llm

import (
	"testing"
)

func TestEstimateTokens_Empty(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Errorf("EstimateTokens(\"\") = %d, want 0", got)
	}
}

func TestEstimateTokens_Short(t *testing.T) {
	// "hi" is 2 chars → len/4 = 0, but floor is 1
	if got := EstimateTokens("hi"); got != 1 {
		t.Errorf("EstimateTokens(\"hi\") = %d, want 1", got)
	}
}

func TestEstimateTokens_Long(t *testing.T) {
	// 400 chars → 100 tokens
	text := ""
	for range 100 {
		text += "abcd"
	}
	if got := EstimateTokens(text); got != 100 {
		t.Errorf("EstimateTokens(400-char) = %d, want 100", got)
	}
}

func TestEstimateCost_KnownModel(t *testing.T) {
	// gpt-4o-mini: $0.15 input + $0.60 output per 1M tokens
	// 1M input + 1M output → $0.75
	got := EstimateCost("gpt-4o-mini", 1_000_000, 1_000_000)
	want := 0.75
	if got != want {
		t.Errorf("EstimateCost gpt-4o-mini 1M/1M = %f, want %f", got, want)
	}
}

func TestEstimateCost_UnknownModel(t *testing.T) {
	got := EstimateCost("unknown-model-xyz", 1_000_000, 1_000_000)
	if got != 0 {
		t.Errorf("EstimateCost unknown model = %f, want 0", got)
	}
}
