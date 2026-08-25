package server

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
)

// TestWriteDebugError_SetsJSONContentType regression-guards: writeDebugError
// used http.Error for every branch, which unconditionally sets
// Content-Type: text/plain even though the body is always a JSON literal —
// a caller that trusts the header over sniffing the body mishandles the
// response.
func TestWriteDebugError_SetsJSONContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	writeDebugError(rec, errAmbiguousSession)

	got := rec.Header().Get("Content-Type")
	if got != "application/json" && got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

// TestWriteDebugError_EscapesErrorMessage regression-guards: the default
// branch built the body via raw string concatenation
// (`{"error":"`+err.Error()+`"}`) with no escaping — an error message
// containing a `"`, `\`, or newline produced invalid JSON, so a client's
// JSON.parse/json.Unmarshal on it failed with a confusing parse error
// instead of surfacing the actual message.
func TestWriteDebugError_EscapesErrorMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	writeDebugError(rec, errors.New(`quote " backslash \ newline` + "\n" + "end"))

	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not valid JSON: %v — body: %s", err, rec.Body.String())
	}
}
