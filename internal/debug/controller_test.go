package debug

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/paupawsan/rakitsu/internal/telemetry"
)

func TestNewDebugController_StartsRunning(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)
	if dc.state != StateRunning {
		t.Errorf("expected StateRunning, got %q", dc.state)
	}
}

func TestBreakpoint_PausesAtMatch(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)
	dc.SetBreakpoint(BreakpointKey{EventType: "pre_agent", AgentName: "Worker"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Check should block (pause)
	var paused bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := dc.Check(ctx, "pre_agent", "Worker", 0, nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		paused = true
	}()

	// Give it time to pause
	time.Sleep(100 * time.Millisecond)

	// Resume
	dc.Resume(ActionResume)
	wg.Wait()

	if !paused {
		t.Error("expected agent to have been paused and resumed")
	}
}

func TestBreakpoint_DoesNotPauseOnMismatch(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)
	dc.SetBreakpoint(BreakpointKey{EventType: "pre_agent", AgentName: "Worker"})

	ctx := context.Background()
	// Different agent — should NOT pause
	err := dc.Check(ctx, "pre_agent", "Other", 0, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBreakpoint_EventTypeAlias(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)
	// Set breakpoint with event type name (frontend sends this)
	dc.SetBreakpoint(BreakpointKey{EventType: "AGENT_START", AgentName: "Worker"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var paused bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Check uses checkpoint name (Go code sends this)
		dc.Check(ctx, "pre_agent", "Worker", 0, nil)
		paused = true
	}()

	time.Sleep(100 * time.Millisecond)
	dc.Resume(ActionResume)
	wg.Wait()

	if !paused {
		t.Error("expected AGENT_START alias to match pre_agent checkpoint")
	}
}

func TestResume_TransitionsToRunning(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)
	dc.SetBreakpoint(BreakpointKey{EventType: "pre_agent", AgentName: "*"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		dc.Check(ctx, "pre_agent", "Worker", 0, nil)
	}()

	time.Sleep(100 * time.Millisecond)

	// After pause, state should be paused
	dc.mu.RLock()
	if dc.state != StatePaused {
		t.Errorf("expected StatePaused, got %q", dc.state)
	}
	dc.mu.RUnlock()

	dc.Resume(ActionResume)
	wg.Wait()

	// After resume, state should be running
	dc.mu.RLock()
	if dc.state != StateRunning {
		t.Errorf("expected StateRunning after resume, got %q", dc.state)
	}
	dc.mu.RUnlock()
}

func TestResume_AfterResumeDoesNotPauseWithoutBreakpoint(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)
	dc.SetBreakpoint(BreakpointKey{EventType: "pre_agent", AgentName: "First"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// First check: pauses at "First"
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		dc.Check(ctx, "pre_agent", "First", 0, nil)
	}()
	time.Sleep(100 * time.Millisecond)
	dc.Resume(ActionResume)
	wg.Wait()

	// Second check: different agent, no breakpoint — should NOT pause
	done := make(chan struct{})
	go func() {
		dc.Check(ctx, "pre_agent", "Second", 0, nil)
		close(done)
	}()

	select {
	case <-done:
		// Good — didn't pause
	case <-time.After(500 * time.Millisecond):
		t.Error("Check blocked unexpectedly for non-breakpointed agent")
	}
}

func TestWildcardBreakpoint_MatchesAll(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)
	dc.SetBreakpoint(BreakpointKey{EventType: "pre_agent", AgentName: "*"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		dc.Check(ctx, "pre_agent", "AnyAgent", 0, nil)
	}()

	time.Sleep(100 * time.Millisecond)
	dc.Resume(ActionResume)
	wg.Wait()
	// If we get here without timeout, wildcard matched
}

func TestClearBreakpoint(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)
	dc.SetBreakpoint(BreakpointKey{EventType: "pre_agent", AgentName: "Worker"})
	dc.ClearBreakpoint(BreakpointKey{EventType: "pre_agent", AgentName: "Worker"})

	ctx := context.Background()
	// Should NOT pause — breakpoint was cleared
	done := make(chan struct{})
	go func() {
		dc.Check(ctx, "pre_agent", "Worker", 0, nil)
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(500 * time.Millisecond):
		t.Error("Check blocked after breakpoint was cleared")
	}
}

func TestStep_PausesAtNextCheckpoint(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)

	// Manually set to stepping mode
	dc.mu.Lock()
	dc.state = StateStepping
	dc.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		dc.Check(ctx, "pre_thought", "Worker", 0, nil)
	}()

	time.Sleep(100 * time.Millisecond)
	dc.Resume(ActionResume)
	wg.Wait()
}

func TestPreloadedBreakpoints_WorkBeforeRun(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)

	// Simulate breakpoints sent with run request (pre-loaded before goroutine starts)
	dc.SetBreakpoint(BreakpointKey{EventType: "AGENT_START", AgentName: "BackendDesigner"})
	dc.SetBreakpoint(BreakpointKey{EventType: "PIPELINE_STEP_START", AgentName: "backend"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// First checkpoint: pipeline step — should pause
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		dc.Check(ctx, "pre_pipeline_step", "backend", 0, nil)
	}()
	time.Sleep(100 * time.Millisecond)
	dc.Resume(ActionResume)
	wg.Wait()

	// Second checkpoint: agent — should also pause
	wg.Add(1)
	go func() {
		defer wg.Done()
		dc.Check(ctx, "pre_agent", "BackendDesigner", 0, nil)
	}()
	time.Sleep(100 * time.Millisecond)
	dc.Resume(ActionResume)
	wg.Wait()
}

func TestRequestPause_PausesAtNextCheckpoint(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)
	// No breakpoints — normally wouldn't pause

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// First check: no breakpoints, no stepping — should NOT pause
	err := dc.Check(ctx, "pre_agent", "Worker", 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Request pause (like user clicking Pause button)
	dc.RequestPause()

	// Next check: should pause because state is StateStepping
	var paused bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		dc.Check(ctx, "pre_thought", "Worker", 0, nil)
		paused = true
	}()

	time.Sleep(100 * time.Millisecond)

	// Verify it's paused
	dc.mu.RLock()
	state := dc.state
	dc.mu.RUnlock()
	if state != StatePaused {
		t.Errorf("expected StatePaused after RequestPause, got %q", state)
	}

	dc.Resume(ActionResume)
	wg.Wait()

	if !paused {
		t.Error("expected pause after RequestPause")
	}
}

func TestContextCancel_UnblocksPausedAgent(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)
	dc.SetBreakpoint(BreakpointKey{EventType: "pre_agent", AgentName: "Worker"})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// This should pause, then context timeout should unblock it
	err := dc.Check(ctx, "pre_agent", "Worker", 0, nil)
	if err == nil {
		t.Error("expected context error after timeout")
	}
}
