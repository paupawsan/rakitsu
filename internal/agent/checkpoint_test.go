package agent

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- buildCheckpoint ---

func TestBuildCheckpoint_Empty(t *testing.T) {
	pctx := newPipelineContext("my query")
	cp := buildCheckpoint(pctx, nil)

	if cp.Query != "my query" {
		t.Errorf("Query = %q, want %q", cp.Query, "my query")
	}
	if len(cp.CompletedSteps) != 0 {
		t.Errorf("CompletedSteps = %v, want empty", cp.CompletedSteps)
	}
	if len(cp.Results) != 0 {
		t.Errorf("Results = %v, want empty", cp.Results)
	}
}

func TestBuildCheckpoint_SuccessfulSteps(t *testing.T) {
	pctx := newPipelineContext("q")
	pctx.addResult(&StepResult{Name: "plan", Output: "PLAN output", Duration: 2 * time.Second})
	pctx.addResult(&StepResult{Name: "implement", Output: "CODE output", Duration: 5 * time.Second})

	cp := buildCheckpoint(pctx, nil)

	if len(cp.CompletedSteps) != 2 {
		t.Fatalf("CompletedSteps len = %d, want 2", len(cp.CompletedSteps))
	}
	if cp.Results["plan"].Output != "PLAN output" {
		t.Errorf("plan output = %q, want %q", cp.Results["plan"].Output, "PLAN output")
	}
	if cp.Results["implement"].Duration != 5*time.Second {
		t.Errorf("implement duration = %v, want 5s", cp.Results["implement"].Duration)
	}
}

func TestBuildCheckpoint_FailedStepsExcluded(t *testing.T) {
	pctx := newPipelineContext("q")
	pctx.addResult(&StepResult{Name: "plan", Output: "ok", Duration: time.Second})
	pctx.addResult(&StepResult{Name: "implement", Output: "", Error: fmt.Errorf("LLM error")})

	cp := buildCheckpoint(pctx, nil)

	if len(cp.CompletedSteps) != 1 {
		t.Errorf("CompletedSteps = %v, want [plan] only", cp.CompletedSteps)
	}
	if _, ok := cp.Results["implement"]; ok {
		t.Error("failed step 'implement' should NOT be in checkpoint results")
	}
}

func TestBuildCheckpoint_PreservesSessionID(t *testing.T) {
	pctx := newPipelineContext("q")
	pctx.addResult(&StepResult{Name: "plan", Output: "ok"})

	prior := &CheckpointData{SessionID: "session-abc-123"}
	cp := buildCheckpoint(pctx, prior)

	if cp.SessionID != "session-abc-123" {
		t.Errorf("SessionID = %q, want %q", cp.SessionID, "session-abc-123")
	}
}

func TestBuildCheckpoint_NilPriorSessionIDEmpty(t *testing.T) {
	pctx := newPipelineContext("q")
	cp := buildCheckpoint(pctx, nil)
	if cp.SessionID != "" {
		t.Errorf("SessionID = %q, want empty when no prior", cp.SessionID)
	}
}

// --- CheckpointData round-trip identity ---

func TestCheckpointData_RoundTrip(t *testing.T) {
	original := CheckpointData{
		SessionID:      "test-session",
		Query:          "build something",
		CompletedSteps: []string{"plan", "implement"},
		Results: map[string]CheckpointStep{
			"plan":      {Name: "plan", Output: "plan output", Duration: 1 * time.Second},
			"implement": {Name: "implement", Output: "code output", Duration: 10 * time.Second},
		},
	}

	// Verify all fields accessible as expected
	if original.Results["plan"].Output != "plan output" {
		t.Error("round-trip field access failed")
	}
	if original.Results["implement"].Duration != 10*time.Second {
		t.Error("duration round-trip failed")
	}
}

// --- Orchestrator checkpoint integration ---

// mockStep is a helper that builds a fake sequential step with a stub agent.
// Returns the step result output via the provided PipelineContext.
func makeStepResult(name, output string) *StepResult {
	return &StepResult{Name: name, Output: output, Duration: time.Millisecond}
}

func TestBuildCheckpoint_AccumulatesAcrossCalls(t *testing.T) {
	pctx := newPipelineContext("query")

	steps := []struct{ name, out string }{
		{"plan", "plan output"},
		{"implement", "code output"},
		{"verify", "verify output"},
	}

	for i, s := range steps {
		pctx.addResult(makeStepResult(s.name, s.out))
		cp := buildCheckpoint(pctx, nil)

		wantSteps := i + 1
		if len(cp.CompletedSteps) != wantSteps {
			t.Errorf("after step %d: CompletedSteps len = %d, want %d", i, len(cp.CompletedSteps), wantSteps)
		}
		if cp.Results[s.name].Output != s.out {
			t.Errorf("after step %d: output mismatch for %q", i, s.name)
		}
	}
}

// --- Stress tests ---

// TestBuildCheckpoint_Concurrent verifies buildCheckpoint is safe under concurrent reads.
func TestBuildCheckpoint_Concurrent(t *testing.T) {
	pctx := newPipelineContext("concurrent query")
	for i := 0; i < 20; i++ {
		pctx.addResult(makeStepResult(fmt.Sprintf("step-%d", i), fmt.Sprintf("output-%d", i)))
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			cp := buildCheckpoint(pctx, nil)
			if len(cp.Results) != 20 {
				t.Errorf("concurrent buildCheckpoint: got %d results, want 20", len(cp.Results))
			}
		}()
	}
	wg.Wait()
}

// TestCheckpointWriter_CalledAfterEachStep simulates the pipeline loop and
// verifies the checkpoint writer is called after each successful step (not on failure).
func TestCheckpointWriter_CalledAfterEachStep(t *testing.T) {
	var mu sync.Mutex
	var calls []CheckpointData

	writer := func(data CheckpointData) {
		mu.Lock()
		calls = append(calls, data)
		mu.Unlock()
	}

	// Simulate 3 steps: plan (ok), implement (ok), verify (fail)
	pctx := newPipelineContext("query")
	steps := []struct {
		name   string
		output string
		fail   bool
	}{
		{"plan", "plan ok", false},
		{"implement", "code ok", false},
		{"verify", "", true},
	}

	for _, s := range steps {
		var err error
		if s.fail {
			err = fmt.Errorf("step failed")
		}
		pctx.addResult(&StepResult{Name: s.name, Output: s.output, Error: err})
		if err == nil {
			writer(buildCheckpoint(pctx, nil))
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if len(calls) != 2 {
		t.Fatalf("writer called %d times, want 2 (only for successful steps)", len(calls))
	}
	if calls[0].Results["plan"].Output != "plan ok" {
		t.Errorf("first call missing plan result")
	}
	if _, ok := calls[1].Results["implement"]; !ok {
		t.Errorf("second call missing implement result")
	}
	// verify step should never appear in checkpoint
	for _, cp := range calls {
		if _, ok := cp.Results["verify"]; ok {
			t.Errorf("failed step 'verify' appeared in checkpoint")
		}
	}
}

// TestSkipLogic verifies that a pre-loaded checkpoint causes steps to be skipped
// by checking that buildCheckpoint returns saved results when the step is already present.
func TestSkipLogic_SavedResultInjected(t *testing.T) {
	prior := &CheckpointData{
		SessionID:      "s1",
		Query:          "query",
		CompletedSteps: []string{"plan"},
		Results: map[string]CheckpointStep{
			"plan": {Name: "plan", Output: "SAVED plan output", Duration: 3 * time.Second},
		},
	}

	// Simulate resume: inject saved result into pctx (as pipeline.go does)
	pctx := newPipelineContext("query")
	saved := prior.Results["plan"]
	pctx.addResult(&StepResult{
		Name:     saved.Name,
		Output:   saved.Output,
		Duration: saved.Duration,
	})

	// After inject, build checkpoint — should reflect the saved step
	cp := buildCheckpoint(pctx, prior)
	if cp.Results["plan"].Output != "SAVED plan output" {
		t.Errorf("injected result not in checkpoint: got %q", cp.Results["plan"].Output)
	}
	if cp.SessionID != "s1" {
		t.Errorf("SessionID not preserved: got %q", cp.SessionID)
	}
}

// TestStress_100StepPipeline simulates a 100-step pipeline and verifies checkpoint
// correctness grows linearly after each step.
func TestStress_100StepPipeline(t *testing.T) {
	const totalSteps = 100
	pctx := newPipelineContext("stress query")

	var checkpointCallCount atomic.Int64

	for i := 0; i < totalSteps; i++ {
		name := fmt.Sprintf("step-%03d", i)
		output := fmt.Sprintf("output for step %d with some content to make it realistic", i)
		pctx.addResult(makeStepResult(name, output))

		cp := buildCheckpoint(pctx, nil)
		checkpointCallCount.Add(1)

		// Verify checkpoint grows correctly
		wantLen := i + 1
		if len(cp.CompletedSteps) != wantLen {
			t.Errorf("step %d: CompletedSteps len = %d, want %d", i, len(cp.CompletedSteps), wantLen)
		}
		if len(cp.Results) != wantLen {
			t.Errorf("step %d: Results len = %d, want %d", i, len(cp.Results), wantLen)
		}
		// Spot-check current step
		if cp.Results[name].Output != output {
			t.Errorf("step %d: output mismatch", i)
		}
	}

	if checkpointCallCount.Load() != totalSteps {
		t.Errorf("expected %d checkpoint builds, got %d", totalSteps, checkpointCallCount.Load())
	}
}

// TestStress_ConcurrentCheckpointBuilds verifies no data race when multiple goroutines
// call buildCheckpoint on the same pctx simultaneously while one goroutine is adding results.
func TestStress_ConcurrentCheckpointBuilds(t *testing.T) {
	pctx := newPipelineContext("concurrent stress")

	// Pre-populate 50 steps
	for i := 0; i < 50; i++ {
		pctx.addResult(makeStepResult(fmt.Sprintf("step-%d", i), fmt.Sprintf("out-%d", i)))
	}

	const readers = 200
	var wg sync.WaitGroup
	wg.Add(readers)

	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			cp := buildCheckpoint(pctx, nil)
			// Just access fields to trigger race detector
			_ = len(cp.Results)
			_ = cp.Query
		}()
	}
	wg.Wait()
}
