package llm

import (
	"context"
	"testing"
)

// overrideAwareProvider records the override it received from
// GenerateWithOverride so the test can assert dispatch worked.
type overrideAwareProvider struct {
	name           string
	model          string
	plainCalls     int
	overrideCalls  int
	gotOverride    OverrideConfig
}

func (p *overrideAwareProvider) Generate(_ context.Context, _ string, _ []Message, _ []ToolDefinition) (*GenerateResult, error) {
	p.plainCalls++
	return &GenerateResult{Response: "plain"}, nil
}

func (p *overrideAwareProvider) GenerateWithOverride(_ context.Context, _ string, _ []Message, _ []ToolDefinition, override OverrideConfig) (*GenerateResult, error) {
	p.overrideCalls++
	p.gotOverride = override
	return &GenerateResult{Response: "override"}, nil
}

func (p *overrideAwareProvider) GetName() string  { return p.name }
func (p *overrideAwareProvider) GetModel() string { return p.model }

func TestOverridableProvider_DispatchesToOverrideAware(t *testing.T) {
	temp := 0.42
	maxTok := 256
	topP := 0.9
	model := "override-model"

	inner := &overrideAwareProvider{name: "stub", model: "base-model"}
	wrapped := WrapWithOverrides(inner, OverrideConfig{
		Temperature: &temp,
		MaxTokens:   &maxTok,
		TopP:        &topP,
		Model:       &model,
	})

	res, err := wrapped.Generate(context.Background(), "sys", nil, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.Response != "override" {
		t.Errorf("expected override path, got %q", res.Response)
	}
	if inner.overrideCalls != 1 {
		t.Errorf("expected GenerateWithOverride called once, got %d", inner.overrideCalls)
	}
	if inner.plainCalls != 0 {
		t.Errorf("expected Generate not called, got %d", inner.plainCalls)
	}
	if inner.gotOverride.Temperature == nil || *inner.gotOverride.Temperature != temp {
		t.Errorf("temperature not propagated: %+v", inner.gotOverride.Temperature)
	}
	if inner.gotOverride.MaxTokens == nil || *inner.gotOverride.MaxTokens != maxTok {
		t.Errorf("max tokens not propagated: %+v", inner.gotOverride.MaxTokens)
	}
	if inner.gotOverride.TopP == nil || *inner.gotOverride.TopP != topP {
		t.Errorf("top_p not propagated: %+v", inner.gotOverride.TopP)
	}
	if inner.gotOverride.Model == nil || *inner.gotOverride.Model != model {
		t.Errorf("model not propagated: %+v", inner.gotOverride.Model)
	}

	// GetModel still reflects the override (existing behavior preserved).
	if got := wrapped.GetModel(); got != model {
		t.Errorf("GetModel = %q, want %q", got, model)
	}
}

func TestOverridableProvider_FallsBackForLegacyProvider(t *testing.T) {
	// okProvider does not implement OverrideAware — wrapper must call Generate.
	inner := &okProvider{name: "legacy", model: "m", result: &GenerateResult{Response: "legacy"}}
	wrapped := WrapWithOverrides(inner, OverrideConfig{})

	res, err := wrapped.Generate(context.Background(), "", nil, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.Response != "legacy" {
		t.Errorf("expected legacy path, got %q", res.Response)
	}
	if inner.calls.Load() != 1 {
		t.Errorf("expected Generate called once, got %d", inner.calls.Load())
	}
}
