package llm

// ModelPricing holds cost rates for a model in USD per million tokens.
type ModelPricing struct {
	Input  float64 // USD per 1M input tokens
	Output float64 // USD per 1M output tokens
}

// KnownModelPricing is the static cost table for common models.
// Users can override per-model rates via settings.pricing in YAML.
// Values are in USD per 1M tokens. Keep in sync with internal/agent/pricing.go.
var KnownModelPricing = map[string]ModelPricing{
	// OpenAI
	"gpt-4o":       {Input: 2.50, Output: 10.00},
	"gpt-4o-mini":  {Input: 0.15, Output: 0.60},
	"gpt-4.1":      {Input: 2.00, Output: 8.00},
	"gpt-4.1-mini": {Input: 0.40, Output: 1.60},
	"gpt-4.1-nano": {Input: 0.10, Output: 0.40},
	"o3":           {Input: 2.00, Output: 8.00},
	"o3-mini":      {Input: 1.10, Output: 4.40},
	"o4-mini":      {Input: 1.10, Output: 4.40},

	// Anthropic
	"claude-sonnet-4-5": {Input: 3.00, Output: 15.00},
	"claude-sonnet-4-6": {Input: 3.00, Output: 15.00},
	"claude-opus-4-6":   {Input: 15.00, Output: 75.00},
	"claude-haiku-4-5":  {Input: 0.80, Output: 4.00},

	// Gemini
	"gemini-2.5-pro":                 {Input: 1.25, Output: 10.00},
	"gemini-2.5-flash":               {Input: 0.15, Output: 0.60},
	"gemini-3.1-flash-lite-preview":  {Input: 0.075, Output: 0.30},
}

// EstimateTokens returns a rough token count for a string.
// Uses the heuristic 1 token ≈ 4 characters, which is accurate to ±20%
// for English prose. No external tokenizer dependency.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	n := len(text) / 4
	if n == 0 {
		return 1
	}
	return n
}

// EstimateCost returns estimated USD cost given token counts and a model name.
// Returns 0 when the model is not in KnownModelPricing.
func EstimateCost(model string, inputTokens, outputTokens int) float64 {
	p, ok := KnownModelPricing[model]
	if !ok {
		return 0
	}
	return float64(inputTokens)/1e6*p.Input + float64(outputTokens)/1e6*p.Output
}
