package openai

import (
	"errors"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// Regression tests: GPT-5.6 models reject function tools on
// /v1/chat/completions unless reasoning_effort is sent explicitly. rakitsu
// never set the field, so every tool-using agent on gpt-5.6-* failed with
// a 400 before doing any work (verified live 2026-09-12).

func TestApplyReasoningEffort_SetsWhenConfigured(t *testing.T) {
	req := &openai.ChatCompletionRequest{}
	applyReasoningEffort(req, "none")
	if req.ReasoningEffort != "none" {
		t.Fatalf("ReasoningEffort = %q, want %q", req.ReasoningEffort, "none")
	}
}

func TestApplyReasoningEffort_OmitsWhenEmpty(t *testing.T) {
	req := &openai.ChatCompletionRequest{}
	applyReasoningEffort(req, "")
	if req.ReasoningEffort != "" {
		t.Fatalf("ReasoningEffort = %q, want empty (field must be omitted when unset)", req.ReasoningEffort)
	}
}

func TestWrapAPIError_ReasoningEffortHint(t *testing.T) {
	apiErr := &openai.APIError{
		HTTPStatusCode: 400,
		Message:        "Function tools with reasoning_effort are not supported for gpt-5.6-terra in /v1/chat/completions. To use function tools, use /v1/responses or set reasoning_effort to 'none'.",
	}
	wrapped := wrapAPIError("openai stream error", apiErr)
	if wrapped == nil {
		t.Fatal("wrapAPIError returned nil")
	}
	if !strings.Contains(wrapped.Error(), "reasoning_effort: none") {
		t.Errorf("error should hint at the YAML knob, got: %v", wrapped)
	}
	var back *openai.APIError
	if !errors.As(wrapped, &back) {
		t.Errorf("wrapped error must still unwrap to *openai.APIError")
	}
}

func TestWrapAPIError_NoHintForUnrelatedError(t *testing.T) {
	apiErr := &openai.APIError{HTTPStatusCode: 401, Message: "Incorrect API key provided"}
	wrapped := wrapAPIError("openai stream error", apiErr)
	if strings.Contains(wrapped.Error(), "reasoning_effort") {
		t.Errorf("unrelated error must not carry the reasoning_effort hint, got: %v", wrapped)
	}
}
