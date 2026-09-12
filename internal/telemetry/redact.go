package telemetry

import (
	"bytes"
	"encoding/json"
	"regexp"
)

const redactedMask = "[REDACTED]"

// sensitiveArgKey matches tool-call argument keys that commonly carry
// credentials, so their values can be masked before an event reaches a
// telemetry sink (session JSONL, hub stream). Case-insensitive substring
// match, so e.g. "auth_token" and "db_password" are caught too, not just
// exact keys.
var sensitiveArgKey = regexp.MustCompile(`(?i)token|api[_-]?key|password|secret|authorization`)

// RedactEventPayload masks credential-shaped argument values inside an
// event payload before it reaches a persistence or forwarding sink
// (internal/store.SessionStore.WriteEvent, HubClient.forwardEvents).
// Mirrors config.Redacted()'s shape: locate the field(s) that carry
// arguments, mask matched keys anywhere in them (including nested inside
// objects/arrays, e.g. {"headers":{"Authorization":"..."}}), re-encode.
//
// Everything outside the targeted field(s) is left as untouched raw JSON
// bytes — unknown/future fields survive, and numbers inside the targeted
// field are decoded with json.Number so re-encoding doesn't lose
// precision the way plain map[string]interface{} (which decodes all
// numbers as float64) would for large integers.
//
// First-pass scope: TOOL_CALL_START.Arguments and
// THOUGHT_END's IntendedToolCalls[*].Arguments. TOOL_CALL_END.Output/
// .Error is free-form tool output, not a key/value map, and is out of
// scope for this pass.
//
// On any decode error the input is returned unchanged — never drop or
// corrupt an event over a redaction failure.
func RedactEventPayload(eventType EventType, payload json.RawMessage) json.RawMessage {
	switch eventType {
	case EventToolCallStart:
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(payload, &obj); err != nil {
			return payload
		}
		redactJSONField(obj, "arguments")
		out, err := json.Marshal(obj)
		if err != nil {
			return payload
		}
		return out

	case EventThoughtEnd:
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(payload, &obj); err != nil {
			return payload
		}
		if raw, ok := obj["intended_tool_calls"]; ok {
			var calls []map[string]json.RawMessage
			if err := json.Unmarshal(raw, &calls); err == nil {
				for i := range calls {
					redactJSONField(calls[i], "arguments")
				}
				if out, err := json.Marshal(calls); err == nil {
					obj["intended_tool_calls"] = out
				}
			}
		}
		out, err := json.Marshal(obj)
		if err != nil {
			return payload
		}
		return out

	default:
		return payload
	}
}

// redactJSONField decodes obj[field] (a JSON object), recursively masks
// any credential-shaped key in it, and writes the result back into
// obj[field] as re-encoded JSON. No-op if the field is absent or fails to
// decode — the caller's raw bytes for it are left untouched.
func redactJSONField(obj map[string]json.RawMessage, field string) {
	raw, ok := obj[field]
	if !ok {
		return
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // preserve large-integer precision through the round-trip
	var val interface{}
	if err := dec.Decode(&val); err != nil {
		return
	}
	out, err := json.Marshal(redactValue(val))
	if err != nil {
		return
	}
	obj[field] = out
}

// redactValue walks a value decoded with json.Decoder.UseNumber
// (map[string]interface{}, []interface{}, json.Number, string, bool, or
// nil) and returns a copy with any value masked whose containing map key
// looks credential-shaped, at any nesting depth.
func redactValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			if sensitiveArgKey.MatchString(k) {
				out[k] = redactedMask
			} else {
				out[k] = redactValue(val)
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = redactValue(val)
		}
		return out
	default:
		return v
	}
}
