package openai

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// TestIsReasoningModel locks down the same prefix rules go-openai's
// ReasoningValidator uses internally (reasoning_validator.go), so rakitsu's
// request-building agrees with the client-side validation that would
// otherwise reject the request before it ever reaches the network.
func TestIsReasoningModel(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"o1-preview", true},
		{"o1-mini", true},
		{"o3-mini", true},
		{"o4-mini", true},
		{"gpt-5", true},
		{"gpt-5-nano", true},
		{"gpt-5.9-hypothetical", true}, // matches the "gpt-5" prefix, same as go-openai's validator
		{"gpt-4o-mini", false},
		{"gpt-4o", false},
		{"gpt-3.5-turbo", false},
	}
	for _, c := range cases {
		if got := isReasoningModel(c.model); got != c.want {
			t.Errorf("isReasoningModel(%q) = %v, want %v", c.model, got, c.want)
		}
	}
}

// TestSetMaxTokens_ReasoningModelUsesMaxCompletionTokens is the regression
// test for the bug: rakitsu sent MaxTokens for every model, which go-openai's
// ReasoningValidator rejects client-side for o1/o3/o4/gpt-5 models with
// ErrReasoningModelMaxTokensDeprecated ("this model is not supported
// MaxTokens, please use MaxCompletionTokens") — before any HTTP request goes
// out. Verified live 2026-08-29 against gpt-5-nano with a direct OpenAI key.
func TestSetMaxTokens_ReasoningModelUsesMaxCompletionTokens(t *testing.T) {
	req := &openai.ChatCompletionRequest{}
	setMaxTokens(req, "gpt-5-nano", 2048)

	if req.MaxCompletionTokens != 2048 {
		t.Errorf("MaxCompletionTokens = %d, want 2048", req.MaxCompletionTokens)
	}
	if req.MaxTokens != 0 {
		t.Errorf("MaxTokens = %d, want 0 (reasoning models reject it client-side)", req.MaxTokens)
	}
}

// TestSetMaxTokens_ClassicModelUsesMaxTokens is the non-regression case:
// non-reasoning models keep using MaxTokens exactly as before.
func TestSetMaxTokens_ClassicModelUsesMaxTokens(t *testing.T) {
	req := &openai.ChatCompletionRequest{}
	setMaxTokens(req, "gpt-4o-mini", 2048)

	if req.MaxTokens != 2048 {
		t.Errorf("MaxTokens = %d, want 2048", req.MaxTokens)
	}
	if req.MaxCompletionTokens != 0 {
		t.Errorf("MaxCompletionTokens = %d, want 0", req.MaxCompletionTokens)
	}
}

// TestSetMaxTokens_ZeroOrNegativeIsNoop mirrors the pre-existing "only set
// when > 0" behavior — neither field should be touched.
func TestSetMaxTokens_ZeroOrNegativeIsNoop(t *testing.T) {
	req := &openai.ChatCompletionRequest{}
	setMaxTokens(req, "gpt-5-nano", 0)
	setMaxTokens(req, "gpt-4o-mini", -1)

	if req.MaxTokens != 0 || req.MaxCompletionTokens != 0 {
		t.Errorf("req = %+v, want both fields left at zero", req)
	}
}
