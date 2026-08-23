package agent

import (
	"context"
	"testing"
	"time"
)

func TestNewRateLimiter_ZeroDisabled(t *testing.T) {
	if rl := NewRateLimiter(0); rl != nil {
		t.Error("NewRateLimiter(0) should return nil")
	}
	if rl := NewRateLimiter(-1); rl != nil {
		t.Error("NewRateLimiter(-1) should return nil")
	}
}

func TestNewRateLimiter_Interval(t *testing.T) {
	rl := NewRateLimiter(60) // 60 req/min = 1 req/sec
	if rl == nil {
		t.Fatal("expected non-nil limiter")
	}
	if rl.interval != time.Second {
		t.Errorf("interval = %v, want 1s for 60 rpm", rl.interval)
	}
}

func TestRateLimiter_FirstCallImmediate(t *testing.T) {
	rl := NewRateLimiter(6) // 10s interval
	start := time.Now()
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("first call should be immediate, took %v", elapsed)
	}
}

func TestRateLimiter_EnforcesSpacing(t *testing.T) {
	rl := NewRateLimiter(600) // 100ms interval
	ctx := context.Background()

	// First call — immediate
	if err := rl.Wait(ctx); err != nil {
		t.Fatal(err)
	}

	// Second call — should wait ~100ms
	start := time.Now()
	if err := rl.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 80*time.Millisecond {
		t.Errorf("second call should wait ~100ms, took %v", elapsed)
	}
}

func TestRateLimiter_ContextCancellation(t *testing.T) {
	rl := NewRateLimiter(6) // 10s interval
	ctx := context.Background()

	// First call to set last time
	if err := rl.Wait(ctx); err != nil {
		t.Fatal(err)
	}

	// Cancel immediately — second call should return ctx error
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rl.Wait(ctx)
	if err != context.Canceled {
		t.Errorf("Wait with cancelled ctx returned %v, want context.Canceled", err)
	}
}
