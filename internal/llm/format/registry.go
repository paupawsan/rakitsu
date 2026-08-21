package format

import "regexp"

// ModelPattern associates a model-name regex with a format adapter.
// The first matching entry wins; order matters for overlapping patterns.
type ModelPattern struct {
	Pattern *regexp.Regexp
	Adapter ResponseFormat
}

// modelPatterns is the built-in registry of known-quirky models mapped to
// their correct adapters. Add entries here as new model families appear.
//
// Patterns are case-insensitive and match anywhere in the model name, so
// "vllm-nemotron-elastic-30b" matches /nemotron/, "deepseek-r1-distill" matches
// /deepseek-r1|deepseek-reasoner/, etc.
// Adapter composition notes:
//
// All base-tier reasoning adapters are wrapped with ThinkTagInline AND
// ToolCallTagInline as a defensive safety net. Models sometimes emit stray
// <think> tags or <tool_call> XML even on top of their primary convention
// (e.g. nemotron returning reasoning_content with embedded <think> tags in
// edge cases), and the wrappers are no-ops when the tags aren't present.
// This mirrors the pre-Phase-2 behavior of openai/provider.go which ran
// extractOpenAIThinking + parseContentToolCalls unconditionally on every
// finalized result.
//
// Wrapping order matters:
//   1. Innermost: the base adapter that knows the model's primary format
//   2. Middle: ThinkTagInline strips <think> blocks from the content the
//      base adapter produced (harmless if the base already extracted reasoning)
//   3. Outermost: ToolCallTagInline parses <tool_call> XML from what's left
//      AND skips if the base already produced structured tool calls
//
// reasoningWithInlineFallbacks is the standard wrapper for reasoning models.
func reasoningWithInlineFallbacks() ResponseFormat {
	return ToolCallTagInline{Inner: ThinkTagInline{Inner: ReasoningContentField{}}}
}

// standardWithInlineFallbacks is the standard wrapper for non-reasoning models
// that might still emit inline <think> or <tool_call> tags (Qwen3 variants).
func standardWithInlineFallbacks() ResponseFormat {
	return ToolCallTagInline{Inner: ThinkTagInline{Inner: StandardOpenAI{}}}
}

var modelPatterns = []ModelPattern{
	// Reasoning models that emit chain-of-thought on a separate reasoning_content
	// stream field (vLLM/LiteLLM convention). Wrapped with inline-tag fallbacks
	// for defensive safety against models that mix conventions.
	{regexp.MustCompile(`(?i)nemotron`), reasoningWithInlineFallbacks()},
	{regexp.MustCompile(`(?i)deepseek-r1|deepseek-reasoner`), reasoningWithInlineFallbacks()},
	{regexp.MustCompile(`(?i)qwq`), reasoningWithInlineFallbacks()},

	// Qwen3 thinking / coder variants: emit <think> blocks and/or <tool_call>
	// XML tags inline in standard content. The wrappers do all the work —
	// ReasoningContentField is not involved because these models don't use
	// the reasoning_content field.
	{regexp.MustCompile(`(?i)qwen3.*(thinking|coder|tool)`), standardWithInlineFallbacks()},
}

// DetectResult reports both the chosen adapter and how it was chosen, so
// callers can surface the decision via telemetry / tracer / debugger.
type DetectResult struct {
	Adapter  ResponseFormat
	Reason   string // "override" | "pattern_match" | "fallback"
	Resolver string // "registry" (synchronous decision) | "sniffing" (runtime decision deferred)
}

// Detect returns the adapter for a given model name along with the reason.
//
// Resolution order:
//  1. Explicit override (non-empty): look up by adapter Name() from the
//     built-in set. If the override name is unknown, fall through to
//     pattern detection rather than silently breaking.
//  2. Model-name pattern match against the built-in registry.
//  3. Fallback: Sniffing (runtime auto-detection via streaming deltas).
//
// The fallback is Sniffing rather than StandardOpenAI specifically because
// unknown models might be reasoning models we haven't registered yet.
// Sniffing costs a few buffered deltas of latency for known non-reasoning
// models but protects rakitsu from silently dropping reasoning_content
// on new model families. To force StandardOpenAI on an unknown model, set
// `response_format: standard_openai` in the provider config.
func Detect(modelName, override string) ResponseFormat {
	return DetectWithReason(modelName, override).Adapter
}

// DetectWithReason is the richer variant of Detect used by providers that
// want to emit telemetry about the decision. Callers that don't care about
// the reason can use Detect() for brevity.
func DetectWithReason(modelName, override string) DetectResult {
	if override != "" {
		if adapter := lookupByName(override); adapter != nil {
			resolver := "registry"
			if adapter.Name() == "sniffing" {
				resolver = "sniffing"
			}
			return DetectResult{Adapter: adapter, Reason: "override", Resolver: resolver}
		}
		// Unknown override — fall through to pattern detection rather than
		// silently breaking. The provider logs the fallback decision.
	}

	for _, mp := range modelPatterns {
		if mp.Pattern.MatchString(modelName) {
			return DetectResult{Adapter: mp.Adapter, Reason: "pattern_match", Resolver: "registry"}
		}
	}

	return DetectResult{Adapter: Sniffing{}, Reason: "fallback", Resolver: "sniffing"}
}

// lookupByName returns the adapter with the given Name(), or nil if unknown.
// For wrapping adapters (ThinkTagInline, ToolCallTagInline) this returns the
// adapter with its default StandardOpenAI inner. Users who need a specific
// composition should register a custom pattern rather than relying on the
// override string.
func lookupByName(name string) ResponseFormat {
	switch name {
	case "standard_openai":
		return StandardOpenAI{}
	case "reasoning_content_field":
		return ReasoningContentField{}
	case "sniffing":
		return Sniffing{}
	case "think_tag_inline":
		return ThinkTagInline{}
	case "tool_call_tag_inline":
		return ToolCallTagInline{}
	}
	return nil
}

// KnownAdapterNames returns the set of built-in adapter names for
// validation/documentation purposes.
func KnownAdapterNames() []string {
	return []string{
		"standard_openai",
		"reasoning_content_field",
		"sniffing",
		"think_tag_inline",
		"tool_call_tag_inline",
	}
}
