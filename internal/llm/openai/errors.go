package openai

import (
	"errors"
	"fmt"
	"regexp"

	openai "github.com/sashabaranov/go-openai"
)

// statusError wraps a provider-call error while preserving the HTTP status
// code the OpenAI SDK reported, if any, so callers can classify retryability
// from the actual status (see llm.StatusCoder) instead of pattern-matching
// the message text. It also preserves the structured `param` field the API
// names on a 400 (see Param), so callers can react to the specific rejected
// field instead of guessing ahead of time which models support what.
type statusError struct {
	msg   string
	code  int    // 0 when unknown
	param string // "" when the API didn't name one
	err   error
}

func (e *statusError) Error() string   { return e.msg }
func (e *statusError) Unwrap() error   { return e.err }
func (e *statusError) StatusCode() int { return e.code }

// Param reports the request parameter the API named as rejected (e.g.
// "temperature"), and whether one was present at all. Implements the
// unexported paramCoder interface checked by omitRejectedParam.
func (e *statusError) Param() (string, bool) { return e.param, e.param != "" }

// nestedParamPattern matches a `"param": "name"` JSON key-value pair
// anywhere in a string, not just at the top level of a parsed document. See
// paramFromMessage for why this is needed in addition to APIError.Param.
var nestedParamPattern = regexp.MustCompile(`"param"\s*:\s*"([a-zA-Z_]+)"`)

// paramFromMessage falls back to scanning a raw error message for a nested
// `"param": "..."` pair when the SDK's structured APIError.Param is empty.
// Needed because the DGX LiteLLM proxy, for some failure paths, double-
// encodes the upstream provider's own JSON error body as plain text inside
// its own top-level `message` field instead of lifting the upstream's
// `param` field up to its own — observed live: a 400 whose outer envelope
// has `param: null` but whose `message` string contains a full nested
// `{"error": {..., "param": "temperature", ...}}` blob as literal text.
func paramFromMessage(msg string) (string, bool) {
	m := nestedParamPattern.FindStringSubmatch(msg)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// wrapAPIError wraps err (as returned by the go-openai client) with prefix,
// carrying forward its HTTP status code and, when present, its named `param`
// field when err is a recognized SDK error type. Both errors go-openai
// returns on a non-2xx response — APIError (parsed error body) and
// RequestError (fallback for an unparseable body) — carry an HTTPStatusCode
// field; only APIError's body is structured enough to name a param, and even
// then only when the proxy propagated it to its own top level rather than
// burying it in `message` (see paramFromMessage).
func wrapAPIError(prefix string, err error) error {
	code := 0
	param := ""
	var apiErr *openai.APIError
	var reqErr *openai.RequestError
	switch {
	case errors.As(err, &apiErr):
		code = apiErr.HTTPStatusCode
		if apiErr.Param != nil && *apiErr.Param != "" {
			param = *apiErr.Param
		} else if p, ok := paramFromMessage(apiErr.Message); ok {
			param = p
		}
	case errors.As(err, &reqErr):
		code = reqErr.HTTPStatusCode
	}
	return &statusError{msg: fmt.Sprintf("%s: %s", prefix, err), code: code, param: param, err: err}
}
