package agent

import (
	"context"
	"sync"
	"time"
)

// RateLimiter enforces a minimum interval between LLM requests.
// It is provider-scoped: multiple agents sharing a provider share one limiter.
type RateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
}

// NewRateLimiter creates a rate limiter for the given requests-per-minute.
// A value of 0 or negative disables limiting (Wait returns immediately).
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	if requestsPerMinute <= 0 {
		return nil
	}
	return &RateLimiter{
		interval: time.Minute / time.Duration(requestsPerMinute),
	}
}

// Wait blocks until the next request is allowed or ctx is cancelled.
func (r *RateLimiter) Wait(ctx context.Context) error {
	r.mu.Lock()
	now := time.Now()
	next := r.last.Add(r.interval)
	if now.Before(next) {
		delay := next.Sub(now)
		r.last = next
		r.mu.Unlock()
		select {
		case <-time.After(delay):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r.last = now
	r.mu.Unlock()
	return nil
}
