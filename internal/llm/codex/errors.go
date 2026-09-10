package codex

import "fmt"

// statusError carries the HTTP status the Codex backend returned so the
// agent retry logic can classify it via llm.StatusCoder instead of parsing
// the message text.
type statusError struct {
	msg  string
	code int
}

func (e *statusError) Error() string   { return e.msg }
func (e *statusError) StatusCode() int { return e.code }

func newStatusError(code int, format string, args ...interface{}) *statusError {
	return &statusError{msg: fmt.Sprintf(format, args...), code: code}
}
