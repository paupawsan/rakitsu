package config

import "testing"

// reasoning_effort must be settable per provider and per agent
// (model_config), with the agent-level value winning.

func TestGetReasoningEffort_FromProvider(t *testing.T) {
	cfg := Config{Settings: Settings{Providers: map[string]ProviderDefinition{
		"openai": {Type: "openai", ReasoningEffort: "none"},
	}}}
	if got := cfg.GetReasoningEffort("openai"); got != "none" {
		t.Fatalf("GetReasoningEffort(openai) = %q, want %q", got, "none")
	}
	if got := cfg.GetReasoningEffort("missing"); got != "" {
		t.Fatalf("GetReasoningEffort(missing) = %q, want empty", got)
	}
}

func TestModelConfig_ReasoningEffortField(t *testing.T) {
	mc := ModelConfig{ReasoningEffort: "low"}
	if mc.ReasoningEffort != "low" {
		t.Fatalf("ModelConfig.ReasoningEffort = %q, want %q", mc.ReasoningEffort, "low")
	}
}
