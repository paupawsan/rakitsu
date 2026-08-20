package format

import "encoding/json"

// parseJSONArgs parses a tool-call arguments JSON string into a map.
// Returns (nil, false) on parse failure so callers can decide whether to
// surface the raw string or drop the args.
func parseJSONArgs(s string) (map[string]interface{}, bool) {
	if s == "" {
		return nil, false
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(s), &args); err != nil {
		return nil, false
	}
	return args, true
}
