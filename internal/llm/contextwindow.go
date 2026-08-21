package llm

import "strings"

// contextWindows maps known model name prefixes to their context window
// size in tokens. Map iteration order is unspecified in Go, so entry order
// here is not load-bearing — LookupContextWindow explicitly tracks
// len(prefix) > len(bestPrefix) to find the longest/most-specific match
// regardless of order.
var contextWindows = map[string]int{
	// OpenAI
	"gpt-4o-mini":  128000,
	"gpt-4o":       128000,
	"gpt-4.1-nano": 1000000,
	"gpt-4.1-mini": 1000000,
	"gpt-4.1":      1000000,
	"o3-mini":      200000,
	"o3":           200000,
	"o4-mini":      200000,

	// Anthropic
	"claude-sonnet-4-5": 200000,
	"claude-opus-4-6":   200000,
	"claude-haiku-4-5":  200000,

	// Gemini
	"gemini-2.5-pro":   1000000,
	"gemini-2.5-flash": 1000000,
	"gemini-3":         1000000, // covers gemini-3.x-* family by prefix
}

// LookupContextWindow returns the known context window size (in tokens) for
// a model, matched by longest prefix. known=false means the model isn't in
// this table — most often a local (Ollama) or custom (LiteLLM proxy) route —
// and the caller must show "unknown," never a guessed number.
func LookupContextWindow(model string) (int, bool) {
	bestPrefix := ""
	bestSize := 0
	for prefix, size := range contextWindows {
		if strings.HasPrefix(model, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
			bestSize = size
		}
	}
	if bestPrefix == "" {
		return 0, false
	}
	return bestSize, true
}
