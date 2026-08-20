package llm

import "context"

// OverrideConfig holds parameter overrides for a single LLM call.
type OverrideConfig struct {
	Temperature *float64
	MaxTokens   *int
	TopP        *float64
	Model       *string
}

// OverrideAware is an optional interface for providers that can apply
// per-call parameter overrides (Temperature/TopP/MaxTokens) without being
// rebuilt. Providers implement this so the debug controller can experiment
// with sampling parameters mid-run.
type OverrideAware interface {
	GenerateWithOverride(
		ctx context.Context,
		systemPrompt string,
		history []Message,
		tools []ToolDefinition,
		override OverrideConfig,
	) (*GenerateResult, error)
}

// OverridableProvider wraps an LLMProvider and applies parameter overrides
// for the next Generate call. Used by the debug controller for parameter experimentation.
type OverridableProvider struct {
	inner    LLMProvider
	override OverrideConfig
}

// WrapWithOverrides creates a temporary provider wrapper that applies overrides.
func WrapWithOverrides(provider LLMProvider, ovr OverrideConfig) LLMProvider {
	return &OverridableProvider{
		inner:    provider,
		override: ovr,
	}
}

func (o *OverridableProvider) Generate(ctx context.Context, systemPrompt string, history []Message, tools []ToolDefinition) (*GenerateResult, error) {
	// If the underlying provider knows how to apply per-call overrides
	// (Temperature/TopP/MaxTokens), dispatch to that path. Otherwise the
	// override is best-effort: Model is reflected via GetModel and the
	// remaining fields silently no-op.
	if oa, ok := o.inner.(OverrideAware); ok {
		return oa.GenerateWithOverride(ctx, systemPrompt, history, tools, o.override)
	}
	return o.inner.Generate(ctx, systemPrompt, history, tools)
}

func (o *OverridableProvider) GetName() string {
	return o.inner.GetName()
}

func (o *OverridableProvider) GetModel() string {
	if o.override.Model != nil {
		return *o.override.Model
	}
	return o.inner.GetModel()
}

// GetOverride returns the override config for this wrapper.
func (o *OverridableProvider) GetOverride() OverrideConfig {
	return o.override
}
