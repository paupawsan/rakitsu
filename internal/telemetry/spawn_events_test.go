package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentStartPayloadParentAgent(t *testing.T) {
	b, err := json.Marshal(AgentStartPayload{Role: "worker", ParentAgent: "Coordinator"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"parent_agent":"Coordinator"`) {
		t.Errorf("parent_agent missing from JSON: %s", b)
	}
	b, _ = json.Marshal(AgentStartPayload{Role: "worker"})
	if strings.Contains(string(b), "parent_agent") {
		t.Errorf("empty parent_agent should be omitted: %s", b)
	}
}

func TestEventBusCountsDrops(t *testing.T) {
	eb := NewEventBus(1)
	_ = eb.Subscribe() // never drained
	for i := 0; i < 3; i++ {
		eb.Emit("a", EventAgentStart, AgentStartPayload{Role: "worker"})
	}
	if got := eb.DroppedEvents(); got < 2 {
		t.Errorf("DroppedEvents = %d, want >= 2", got)
	}
}

func TestTracerParentDepth(t *testing.T) {
	tr := &ConsoleTracer{agentDepth: map[string]int{"Coordinator": 0}, depth: 5}
	if d := tr.parentDepth(AgentEvent{ParentID: "Coordinator"}); d != 1 {
		t.Errorf("parentDepth = %d, want 1", d)
	}
	if d := tr.parentDepth(AgentEvent{}); d != -1 {
		t.Errorf("parentDepth no-parent = %d, want -1", d)
	}
	if d := tr.parentDepth(AgentEvent{ParentID: "Unknown"}); d != -1 {
		t.Errorf("parentDepth unknown = %d, want -1", d)
	}
}
