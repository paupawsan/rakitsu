package openai

import (
	"errors"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

// statusError wraps a provider-call error while preserving the HTTP status
// code the OpenAI SDK reported, if any, so callers can classify retryability
// from the actual status (see llm.StatusCoder) instead of pattern-matching
// the message text.
type statusError struct {
	msg  string
	code int // 0 when unknown
	err  error
}

func (e *statusError) Error() string   { return e.msg }
func (e *statusError) Unwrap() error   { return e.err }
func (e *statusError) StatusCode() int { return e.code }

// wrapAPIError wraps err (as returned by the go-openai client) with prefix,
// carrying forward its HTTP status code when err is a recognized SDK error
// type. Both errors go-openai returns on a non-2xx response — APIError
// (parsed error body) and RequestError (fallback for an unparseable body) —
// carry an HTTPStatusCode field.
func wrapAPIError(prefix string, err error) error {
	code := 0
	var apiErr *openai.APIError
	var reqErr *openai.RequestError
	switch {
	case errors.As(err, &apiErr):
		code = apiErr.HTTPStatusCode
	case errors.As(err, &reqErr):
		code = reqErr.HTTPStatusCode
	}
	return &statusError{msg: fmt.Sprintf("%s: %s", prefix, err), code: code, err: err}
}
