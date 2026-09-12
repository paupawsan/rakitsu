package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactEventPayload_ToolCallStart_MasksSensitiveArgs(t *testing.T) {
	payload, _ := json.Marshal(ToolCallStartPayload{
		ToolCallID: "tc-1",
		ToolName:   "cli",
		Arguments: map[string]interface{}{
			"command":  "curl",
			"token":    "sk-secret",
			"api_key":  "abc123",
			"password": "hunter2",
		},
	})

	out := RedactEventPayload(EventToolCallStart, payload)

	var got ToolCallStartPayload
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal redacted payload: %v", err)
	}
	if got.Arguments["command"] != "curl" {
		t.Fatalf("non-sensitive arg was altered: %+v", got.Arguments)
	}
	for _, key := range []string{"token", "api_key", "password"} {
		if got.Arguments[key] != redactedMask {
			t.Fatalf("arg %q not redacted: %+v", key, got.Arguments)
		}
	}
}

func TestRedactEventPayload_ThoughtEnd_MasksIntendedToolCallArgs(t *testing.T) {
	payload, _ := json.Marshal(ThoughtEndPayload{
		StructuredThought: StructuredThought{
			Reasoning: "calling the api",
			IntendedToolCalls: []ToolCallSignature{
				{Name: "http", Arguments: map[string]interface{}{
					"url":           "https://example.com",
					"Authorization": "Bearer xyz",
				}},
			},
		},
		Iteration: 1,
	})

	out := RedactEventPayload(EventThoughtEnd, payload)

	var got ThoughtEndPayload
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal redacted payload: %v", err)
	}
	args := got.IntendedToolCalls[0].Arguments
	if args["url"] != "https://example.com" {
		t.Fatalf("non-sensitive arg was altered: %+v", args)
	}
	if args["Authorization"] != redactedMask {
		t.Fatalf("Authorization arg not redacted: %+v", args)
	}
}

// TestRedactEventPayload_NestedSensitiveKeys_Masked regression-guards a
// finding from code review: redaction only walked the
// top level of Arguments, so a credential-shaped key nested inside an
// object or array (e.g. {"headers":{"Authorization":"..."}}) reached the
// sink unredacted.
func TestRedactEventPayload_NestedSensitiveKeys_Masked(t *testing.T) {
	payload := json.RawMessage(`{
		"tool_call_id": "tc-1",
		"tool_name": "http",
		"arguments": {
			"url": "https://example.com",
			"headers": {"Authorization": "Bearer xyz", "Accept": "application/json"},
			"retries": [{"api_key": "abc"}, {"note": "fine"}]
		}
	}`)

	out := RedactEventPayload(EventToolCallStart, payload)

	var got struct {
		Arguments struct {
			URL     string              `json:"url"`
			Headers map[string]string   `json:"headers"`
			Retries []map[string]string `json:"retries"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal redacted payload: %v", err)
	}
	if got.Arguments.URL != "https://example.com" {
		t.Fatalf("non-sensitive arg was altered: %+v", got.Arguments)
	}
	if got.Arguments.Headers["Authorization"] != redactedMask {
		t.Fatalf("nested Authorization not redacted: %+v", got.Arguments.Headers)
	}
	if got.Arguments.Headers["Accept"] != "application/json" {
		t.Fatalf("nested non-sensitive header altered: %+v", got.Arguments.Headers)
	}
	if got.Arguments.Retries[0]["api_key"] != redactedMask {
		t.Fatalf("nested-in-array api_key not redacted: %+v", got.Arguments.Retries)
	}
	if got.Arguments.Retries[1]["note"] != "fine" {
		t.Fatalf("nested-in-array non-sensitive value altered: %+v", got.Arguments.Retries)
	}
}

// TestRedactEventPayload_PreservesLargeIntegerPrecision regression-guards
// a finding from code review: decoding Arguments
// through a plain map[string]interface{} turns every JSON number into a
// float64, which loses precision above 2^53 and silently changes a
// non-sensitive argument's value on re-encode.
func TestRedactEventPayload_PreservesLargeIntegerPrecision(t *testing.T) {
	payload := json.RawMessage(`{"tool_call_id":"tc-1","tool_name":"cli","arguments":{"offset":9007199254740993}}`)

	out := RedactEventPayload(EventToolCallStart, payload)

	if !strings.Contains(string(out), `"offset":9007199254740993`) {
		t.Fatalf("large integer argument lost precision: %s", out)
	}
}

// TestRedactEventPayload_PreservesUnknownFields regression-guards the
// review's secondary point: decoding into the typed payload struct drops
// any field the struct doesn't declare. Operating on the raw JSON object
// instead must carry every field through untouched.
func TestRedactEventPayload_PreservesUnknownFields(t *testing.T) {
	payload := json.RawMessage(`{"tool_call_id":"tc-1","tool_name":"cli","arguments":{"token":"sk-1"},"future_field":"kept"}`)

	out := RedactEventPayload(EventToolCallStart, payload)

	if !strings.Contains(string(out), `"future_field":"kept"`) {
		t.Fatalf("unknown field was dropped: %s", out)
	}
	if !strings.Contains(string(out), `"token":"`+redactedMask+`"`) {
		t.Fatalf("token still not redacted: %s", out)
	}
}

func TestRedactEventPayload_UnrelatedEventType_ReturnsUnchanged(t *testing.T) {
	payload := json.RawMessage(`{"output":"contains a token=sk-secret but is out of scope"}`)

	out := RedactEventPayload(EventToolCallEnd, payload)

	if string(out) != string(payload) {
		t.Fatalf("TOOL_CALL_END payload should pass through unchanged, got %s", out)
	}
}

func TestRedactEventPayload_MalformedPayload_ReturnsUnchanged(t *testing.T) {
	payload := json.RawMessage(`not json`)

	out := RedactEventPayload(EventToolCallStart, payload)

	if string(out) != string(payload) {
		t.Fatalf("malformed payload should pass through unchanged, got %s", out)
	}
}
