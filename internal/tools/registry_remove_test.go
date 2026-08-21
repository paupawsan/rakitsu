package tools

import (
	"context"
	"testing"
)

type stubTool struct{ name string }

func (s *stubTool) GetName() string        { return s.name }
func (s *stubTool) GetDescription() string { return "stub" }
func (s *stubTool) GetParametersSchema() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (s *stubTool) Execute(context.Context, map[string]interface{}) (string, error) {
	return "ok", nil
}

func TestRemoveTool(t *testing.T) {
	r := NewToolRegistry()
	r.RegisterTool(&stubTool{name: "keep"})
	r.RegisterTool(&stubTool{name: "spawn_agent"})

	r.RemoveTool("spawn_agent")

	if r.GetTool("spawn_agent") != nil {
		t.Error("spawn_agent still resolvable after RemoveTool")
	}
	if r.GetTool("keep") == nil {
		t.Error("RemoveTool removed the wrong tool")
	}
	defs := r.GetToolDefinitions()
	if len(defs) != 1 {
		t.Fatalf("GetToolDefinitions() returned %d entries, want 1", len(defs))
	}
	if defs[0].Function.Name != "keep" {
		t.Errorf("GetToolDefinitions() = %q, want [\"keep\"]", defs[0].Function.Name)
	}
}

func TestRemoveToolUnknownIsNoOp(t *testing.T) {
	r := NewToolRegistry()
	r.RegisterTool(&stubTool{name: "keep"})
	r.RemoveTool("never_registered")
	if len(r.GetAllTools()) != 1 {
		t.Errorf("registry size = %d, want 1", len(r.GetAllTools()))
	}
}
