package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDirectChatEventTypeStrings(t *testing.T) {
	if EventDirectChatStart != "DIRECT_CHAT_START" {
		t.Errorf("EventDirectChatStart = %q, want DIRECT_CHAT_START", EventDirectChatStart)
	}
	if EventDirectChatEnd != "DIRECT_CHAT_END" {
		t.Errorf("EventDirectChatEnd = %q, want DIRECT_CHAT_END", EventDirectChatEnd)
	}
}

func TestDirectChatPayloadsRoundTrip(t *testing.T) {
	start := DirectChatStartPayload{Agent: "Researcher-1", Kind: "spawned", Message: "why skip the API docs?"}
	data, err := json.Marshal(start)
	if err != nil {
		t.Fatalf("marshal start: %v", err)
	}
	var gotStart DirectChatStartPayload
	if err := json.Unmarshal(data, &gotStart); err != nil {
		t.Fatalf("unmarshal start: %v", err)
	}
	if gotStart != start {
		t.Errorf("start round-trip = %+v, want %+v", gotStart, start)
	}
	if string(data) != `{"agent":"Researcher-1","kind":"spawned","message":"why skip the API docs?"}` {
		t.Errorf("start json = %s", data)
	}

	end := DirectChatEndPayload{Agent: "Researcher-1", Kind: "spawned", Reply: "the task named only the README", Tokens: 412, Status: "success"}
	data, err = json.Marshal(end)
	if err != nil {
		t.Fatalf("marshal end: %v", err)
	}
	var gotEnd DirectChatEndPayload
	if err := json.Unmarshal(data, &gotEnd); err != nil {
		t.Fatalf("unmarshal end: %v", err)
	}
	if gotEnd != end {
		t.Errorf("end round-trip = %+v, want %+v", gotEnd, end)
	}
	// Error is omitempty — a successful end must not carry an empty error key.
	if strings.Contains(string(data), `"error"`) {
		t.Errorf("successful end payload should omit error, got %s", data)
	}
}

func TestDirectChatEventsPublishThroughBus(t *testing.T) {
	bus := NewEventBus(8)
	sub := bus.Subscribe()
	bus.Emit("Researcher-1", EventDirectChatStart, DirectChatStartPayload{
		Agent: "Researcher-1", Kind: "spawned", Message: "hi",
	})
	bus.Unsubscribe(sub)

	var seen []AgentEvent
	for ev := range sub {
		seen = append(seen, ev)
	}
	if len(seen) != 1 {
		t.Fatalf("got %d events, want 1", len(seen))
	}
	if seen[0].EventType != EventDirectChatStart {
		t.Errorf("event type = %q, want %q", seen[0].EventType, EventDirectChatStart)
	}
}
