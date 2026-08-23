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

// TestResume_IgnoredWhenNotPaused regression-guards finding 23: Resume()
// used to rely solely on "select with default" against resumeCh (buffered,
// capacity 1) to detect "not paused". A buffered send succeeds even with no
// waiting receiver, so a stale/late Resume() call issued while the
// controller was StateRunning would silently land in the buffer instead of
// being discarded — then get consumed as a spurious pre-armed resume the
// NEXT time Check() actually paused, skipping that pause instead of
// blocking for a real decision.
func TestResume_IgnoredWhenNotPaused(t *testing.T) {
	bus := telemetry.NewEventBus(8)
	dc := NewDebugController(bus)
	dc.SetBreakpoint(BreakpointKey{EventType: "pre_agent", AgentName: "Worker"})

	// Fire a Resume() while nothing is paused — must be discarded, not
	// buffered into resumeCh.
	dc.Resume(ActionResume)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		dc.Check(ctx, "pre_agent", "Worker", 0, nil)
		close(done)
	}()

	// The stray Resume() above must NOT have pre-armed the pause: Check
	// should still be blocked waiting for a real decision.
	select {
	case <-done:
		t.Fatal("Check returned immediately — a stale Resume() call before the pause was incorrectly consumed by it")
	case <-time.After(150 * time.Millisecond):
		// Good — still paused.
	}

	// A genuine Resume() issued now must still unblock it.
	dc.Resume(ActionResume)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Check never unblocked after a real Resume() call")
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

// TestResume_DuplicateCallDoesNotPreArmTheNextPause regression-guards: the
// GetState() check and the resumeCh send in Resume() used to be two
// separate synchronized steps, so two Resume() calls arriving close
// together for the same pause could both observe StatePaused before either
// send took effect. The second call would then buffer a spurious action
// that the *next* pause's Check() call reads immediately instead of
// actually waiting for a real decision — silently skipping that pause.
// Races two concurrent Resume() calls against one pause across repeated
// trials, then verifies the following pause genuinely blocks (not
// pre-armed) until it gets its own, explicit Resume().
func TestResume_DuplicateCallDoesNotPreArmTheNextPause(t *testing.T) {
	bus := telemetry.NewEventBus(64)
	dc := NewDebugController(bus)

	const trials = 50
	for i := 0; i < trials; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		// First pause: two Resume() calls race to claim it.
		dc.RequestPause()
		done1 := make(chan struct{})
		go func() {
			defer close(done1)
			dc.Check(ctx, "pre_thought", "Worker", i, nil)
		}()
		time.Sleep(20 * time.Millisecond) // let it reach the paused, blocking state

		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		for j := 0; j < 2; j++ {
			go func() {
				defer wg.Done()
				<-start
				dc.Resume(ActionResume)
			}()
		}
		close(start)
		wg.Wait()

		select {
		case <-done1:
		case <-time.After(time.Second):
			t.Fatalf("trial %d: first Check() never returned after Resume", i)
		}

		// Second, independent pause: must genuinely block — a leaked
		// spurious action from the race above would let it return
		// immediately instead of waiting for its own Resume() call.
		dc.RequestPause()
		done2 := make(chan struct{})
		go func() {
			defer close(done2)
			dc.Check(ctx, "pre_thought", "Worker", i, nil)
		}()

		select {
		case <-done2:
			t.Fatalf("trial %d: second Check() returned without an explicit Resume() — a spurious action leaked from the duplicate-Resume race", i)
		case <-time.After(80 * time.Millisecond):
			// Correct: still blocked, waiting for its own resume signal.
		}

		dc.Resume(ActionResume)
		select {
		case <-done2:
		case <-time.After(time.Second):
			t.Fatalf("trial %d: second Check() never returned after its own Resume", i)
		}

		cancel()
	}
}
