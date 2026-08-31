// Package mocktools provides mock tool implementations for stress testing.
package mocktools

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// EchoTool returns its input as output.
type EchoTool struct{}

func (t *EchoTool) GetName() string        { return "echo" }
func (t *EchoTool) GetDescription() string  { return "Echoes input back" }
func (t *EchoTool) GetParametersSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{"type": "string"},
		},
	}
}
func (t *EchoTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if v, ok := args["input"]; ok {
		return fmt.Sprintf("echo: %v", v), nil
	}
	return "echo: (empty)", nil
}

// SlowTool has configurable latency per call.
type SlowTool struct {
	Name    string
	Latency time.Duration
}

func (t *SlowTool) GetName() string        { return t.Name }
func (t *SlowTool) GetDescription() string  { return "Slow tool with configurable latency" }
func (t *SlowTool) GetParametersSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{"type": "string"},
		},
	}
}
func (t *SlowTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	select {
	case <-time.After(t.Latency):
		return fmt.Sprintf("slow result after %v", t.Latency), nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// LargeOutputTool returns a payload of configurable size.
type LargeOutputTool struct {
	Name string
	Size int // bytes
}

func (t *LargeOutputTool) GetName() string        { return t.Name }
func (t *LargeOutputTool) GetDescription() string  { return "Returns large output" }
func (t *LargeOutputTool) GetParametersSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{"type": "string"},
		},
	}
}
func (t *LargeOutputTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	return strings.Repeat("X", t.Size), nil
}

// FailingTool fails with configurable probability.
type FailingTool struct {
	Name        string
	FailureRate float64 // 0.0-1.0
}

func (t *FailingTool) GetName() string        { return t.Name }
func (t *FailingTool) GetDescription() string  { return "Tool that sometimes fails" }
func (t *FailingTool) GetParametersSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{"type": "string"},
		},
	}
}
func (t *FailingTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if rand.Float64() < t.FailureRate {
		return "", fmt.Errorf("tool execution failed (simulated)")
	}
	return "success", nil
}
