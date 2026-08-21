package tokenizer

import (
	"fmt"
	"strings"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

// TokenResult holds a single token's ID and text representation.
type TokenResult struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// TokenizeResponse is the API response for a tokenize request.
type TokenizeResponse struct {
	Tokens   []TokenResult `json:"tokens"`
	Total    int           `json:"total"`
	Encoding string        `json:"encoding"`
}

// knownEncodings maps model prefixes to tiktoken encoding names.
// tiktoken-go handles most OpenAI models natively; this covers fallbacks.
var fallbackEncoding = "cl100k_base"

// Tokenize breaks text into individual tokens using the appropriate encoding
// for the given model. For unknown models it falls back to cl100k_base.
func Tokenize(text, model string) (*TokenizeResponse, error) {
	if text == "" {
		return &TokenizeResponse{Tokens: []TokenResult{}, Total: 0, Encoding: ""}, nil
	}

	enc, encodingName, approx := resolveEncoding(model)
	if enc == nil {
		return nil, fmt.Errorf("failed to load encoding for model %q", model)
	}

	ids := enc.Encode(text, nil, nil)

	tokens := make([]TokenResult, len(ids))
	for i, id := range ids {
		decoded := enc.Decode([]int{id})
		tokens[i] = TokenResult{ID: id, Text: decoded}
	}

	label := encodingName
	if approx {
		label += " (approx)"
	}

	return &TokenizeResponse{
		Tokens:   tokens,
		Total:    len(tokens),
		Encoding: label,
	}, nil
}

// resolveEncoding returns the tiktoken encoding, its name, and whether it's approximate.
func resolveEncoding(model string) (*tiktoken.Tiktoken, string, bool) {
	// Try model-specific encoding first (tiktoken-go has built-in mappings)
	enc, err := tiktoken.EncodingForModel(model)
	if err == nil {
		return enc, resolvedEncodingName(model), false
	}

	// Fallback for unknown models (Claude, Gemini, Llama, etc.)
	enc, err = tiktoken.GetEncoding(fallbackEncoding)
	if err != nil {
		return nil, "", false
	}
	return enc, fallbackEncoding, true
}

// resolvedEncodingName mirrors tiktoken.EncodingForModel's own resolution
// order (exact match, then prefix match) so the label always reflects the
// encoding actually returned — including models that only matched via the
// prefix table (e.g. "gpt-4o-mini" via the "gpt-4o-" prefix), which an
// exact-match-only lookup would miss and silently mislabel.
func resolvedEncodingName(model string) string {
	if name, ok := tiktoken.MODEL_TO_ENCODING[model]; ok {
		return name
	}
	for prefix, name := range tiktoken.MODEL_PREFIX_TO_ENCODING {
		if strings.HasPrefix(model, prefix) {
			return name
		}
	}
	return fallbackEncoding
}
