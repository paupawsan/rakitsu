package gemini

import (
	"errors"
	"fmt"

	"google.golang.org/genai"
)

// statusError wraps a provider-call error while preserving the HTTP status
// code the Gemini SDK reported, if any, so callers can classify
// retryability from the actual status (see llm.StatusCoder) instead of
// pattern-matching the message text.
type statusError struct {
	msg  string
	code int // 0 when unknown
	err  error
}

func (e *statusError) Error() string   { return e.msg }
func (e *statusError) Unwrap() error   { return e.err }
func (e *statusError) StatusCode() int { return e.code }

// wrapAPIError wraps err (as returned by the genai client) with prefix,
// carrying forward its HTTP status code when err is a genai.APIError (the
// SDK's type for a response with a non-2xx status).
func wrapAPIError(prefix string, err error) error {
	code := 0
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		code = apiErr.Code
	}
	return &statusError{msg: fmt.Sprintf("%s: %s", prefix, err), code: code, err: err}
}
