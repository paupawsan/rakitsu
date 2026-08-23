package openai

import (
	"errors"
	"testing"

	"github.com/paupawsan/rakitsu/internal/llm"
	openai "github.com/sashabaranov/go-openai"
)

// TestWrapAPIError_ExtractsStatusFromAPIError verifies wrapAPIError carries
// forward the HTTP status code from an *openai.APIError (the SDK's type for
// a response with a parsed JSON error body), and that the result satisfies
// llm.StatusCoder — the interface internal/agent/retry.go relies on to
// classify retryability from the real status instead of pattern-matching
// the message text.
func TestWrapAPIError_ExtractsStatusFromAPIError(t *testing.T) {
	sdkErr := &openai.APIError{
		HTTPStatusCode: 400,
		Message:        "Invalid 'max_tokens': integer below minimum value. Expected a value <= 500.",
	}
	wrapped := wrapAPIError("openai API error", sdkErr)

	var sc llm.StatusCoder
	if !errors.As(wrapped, &sc) {
		t.Fatal("wrapped error does not implement llm.StatusCoder")
	}
	if got := sc.StatusCode(); got != 400 {
		t.Errorf("StatusCode() = %d, want 400", got)
	}
	if !errors.Is(wrapped, sdkErr) {
		t.Error("wrapped error does not unwrap to the original SDK error")
	}
}

// TestWrapAPIError_ExtractsStatusFromRequestError covers the SDK's fallback
// error type (used when the error body isn't parseable JSON), which carries
// the status code under the same field name but a different concrete type.
func TestWrapAPIError_ExtractsStatusFromRequestError(t *testing.T) {
	sdkErr := &openai.RequestError{
		HTTPStatusCode: 503,
		Body:           []byte("<html>upstream error</html>"),
	}
	wrapped := wrapAPIError("openai API error", sdkErr)

	var sc llm.StatusCoder
	if !errors.As(wrapped, &sc) {
		t.Fatal("wrapped error does not implement llm.StatusCoder")
	}
	if got := sc.StatusCode(); got != 503 {
		t.Errorf("StatusCode() = %d, want 503", got)
	}
}

// TestWrapAPIError_UnknownErrorTypeReportsZero verifies an error that isn't
// one of the SDK's known status-carrying types (e.g. a raw network error
// from the underlying transport) reports status 0 rather than a wrong code,
// so callers fall back to text-based classification instead of trusting a
// fabricated status.
func TestWrapAPIError_UnknownErrorTypeReportsZero(t *testing.T) {
	wrapped := wrapAPIError("openai API error", errors.New("dial tcp: connection refused"))

	var sc llm.StatusCoder
	if !errors.As(wrapped, &sc) {
		t.Fatal("wrapped error does not implement llm.StatusCoder")
	}
	if got := sc.StatusCode(); got != 0 {
		t.Errorf("StatusCode() = %d, want 0 (unknown)", got)
	}
}
