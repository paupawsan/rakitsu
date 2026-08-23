package agent

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/paupawsan/rakitsu/internal/config"
)

// RetryConfig controls LLM call retry behaviour.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// Defaults tuned for cloud providers with 60s-window rate limits (NVIDIA NIM
// free tier, OpenAI tier-1, Anthropic dev). With rate-limit-aware backoff
// the curve becomes 10s/20s/40s/60s — 5 attempts span ~130s, enough for one
// 60s rate window to recover. Non-rate-limit retryable errors (500s, network)
// still use BaseDelay (1s → 2s → 4s → 8s → 16s).
var defaultRetryConfig = RetryConfig{
	MaxAttempts: 5,
	BaseDelay:   1 * time.Second,
	MaxDelay:    60 * time.Second,
}

// rateLimitBaseDelay is the starting backoff for 429 / rate-limit errors.
// Longer than BaseDelay because rate-limit windows are typically measured in
// minutes (60s on NVIDIA NIM free, 60s on OpenAI, 60s on Anthropic), not
// seconds like network blips.
const rateLimitBaseDelay = 10 * time.Second

// isRateLimit returns true iff err is specifically a rate-limit signal
// (HTTP 429 or an equivalent textual marker). Callers that need a longer
// backoff curve for 429s key off this rather than the general isRetryable.
func isRateLimit(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests")
}

// isRetryable returns true for transient errors that are worth retrying.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	// Context cancellation is not retryable — caller is shutting down.
	if errors.Is(err, context.Canceled) {
		return false
	}
	if isRateLimit(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	// Server errors
	if strings.Contains(msg, "500") || strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") || strings.Contains(msg, "504") {
		return true
	}
	// Network / timeout
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "eof") || strings.Contains(msg, "broken pipe") {
		return true
	}
	return false
}

// RetryConfigFromSettings builds a RetryConfig from YAML settings.
// Zero values fall back to defaults; legacyAttempts honours the old
// settings.execution.retry_attempts field.
func RetryConfigFromSettings(s config.RetrySettings, legacyAttempts int) RetryConfig {
	cfg := defaultRetryConfig
	if s.MaxAttempts > 0 {
		cfg.MaxAttempts = s.MaxAttempts
	} else if legacyAttempts > 0 {
		cfg.MaxAttempts = legacyAttempts
	}
	if d, err := time.ParseDuration(s.BaseDelay); err == nil && d > 0 {
		cfg.BaseDelay = d
	}
	if d, err := time.ParseDuration(s.MaxDelay); err == nil && d > 0 {
		cfg.MaxDelay = d
	}
	return cfg
}

// RetryEvent describes a pending retry attempt — passed to onRetry
// callbacks so callers can emit telemetry (e.g. EventRetryAttempt) before
// the delay starts. Without this, synthesis-level 429 backoffs would be
// invisible to the TUI / debugger and look like a hang.
type RetryEvent struct {
	Attempt     int           // 1-indexed retry attempt number (0 = first call)
	MaxAttempts int           // total attempts allowed
	Err         error         // last error that triggered the retry
	Delay       time.Duration // about-to-sleep duration
}

// retryWithBackoff calls fn repeatedly with rate-limit-aware exponential
// backoff. Returns nil on first success, the underlying error immediately
// for non-retryable errors, or the last error after cfg.MaxAttempts
// retryable failures. onRetry (optional) fires before each retry sleep so
// callers can emit telemetry; pass nil if not needed.
//
// Currently used by Orchestrator.synthesize (pipeline-level synthesis
// call). Agent.generateWithRetry has its own near-equivalent loop with
// rateLimiter.Wait integration; future cleanup may unify them.
func retryWithBackoff(ctx context.Context, cfg RetryConfig, fn func() error, onRetry func(RetryEvent)) error {
	if cfg.MaxAttempts <= 0 {
		cfg = defaultRetryConfig
	}
	var lastErr error
	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := retryDelay(cfg, attempt-1, isRateLimit(lastErr))
			if onRetry != nil {
				onRetry(RetryEvent{
					Attempt:     attempt,
					MaxAttempts: cfg.MaxAttempts,
					Err:         lastErr,
					Delay:       delay,
				})
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryable(err) {
			return err
		}
	}
	return lastErr
}

// retryDelay returns exponential backoff duration for the given attempt (0-indexed).
// When rateLimited is true, a longer base delay is used because rate-limit
// windows are typically measured in minutes, not seconds.
func retryDelay(cfg RetryConfig, attempt int, rateLimited bool) time.Duration {
	base := cfg.BaseDelay
	if rateLimited {
		base = rateLimitBaseDelay
	}
	raw := float64(base) * math.Pow(2, float64(attempt))
	// Clamp to MaxDelay before int64 conversion to prevent overflow
	if raw > float64(cfg.MaxDelay) || raw < 0 || math.IsInf(raw, 0) {
		return cfg.MaxDelay
	}
	return time.Duration(raw)
}
