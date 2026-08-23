package agent

import (
	"testing"

	"github.com/paupawsan/rakitsu/internal/config"
	"github.com/paupawsan/rakitsu/internal/telemetry"
	"github.com/paupawsan/rakitsu/internal/tools"
	"github.com/paupawsan/rakitsu/internal/tools/fs"
)

func TestGetSystemPrompt(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	a := NewAgent(&config.AgentDefinition{Name: "a", Role: "worker", SystemPrompt: "You are a helpful assistant."}, nil, tools.NewToolRegistry(), bus, nil)
	if got := a.GetSystemPrompt(); got != "You are a helpful assistant." {
		t.Errorf("GetSystemPrompt() = %q, want the configured prompt", got)
	}
}

func TestGetEffectiveToolDefs_ReturnsFullRegistryNotJustDeclaredTools(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	reg := tools.NewToolRegistry()
	reg.RegisterTool(fs.NewTool(&config.ToolDefinition{Name: "fs", Type: "fs", Description: "filesystem access", AllowedPaths: []string{"."}}))
	reg.RegisterTool(fs.NewTool(&config.ToolDefinition{Name: "runtime_only", Type: "fs", Description: "registered at runtime, not in Tools:", AllowedPaths: []string{"."}}))
	// Agent only declares "fs" in its static Tools list — "runtime_only" mirrors
	// how memory_*/user_input/spawn_agent get added straight to the registry
	// without ever appearing in an agent's YAML Tools field.
	a := NewAgent(&config.AgentDefinition{Name: "a", Role: "worker", Tools: []string{"fs"}}, nil, reg, bus, nil)

	defs := a.GetEffectiveToolDefs()
	if len(defs) != 2 {
		t.Fatalf("GetEffectiveToolDefs() returned %d defs, want 2 (full registry, not filtered to declared Tools:): %+v", len(defs), defs)
	}
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["fs"] || !names["runtime_only"] {
		t.Errorf("GetEffectiveToolDefs() = %+v, want both \"fs\" and \"runtime_only\" present", defs)
	}
}
