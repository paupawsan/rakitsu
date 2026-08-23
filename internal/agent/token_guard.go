package agent

import (
	"fmt"
	"sync/atomic"
)

// TokenGuard enforces token and cost budgets.
// Thread-safe via atomic operations.
type TokenGuard struct {
	maxTokens  int64
	maxCost    float64 // USD
	usedTokens atomic.Int64
	usedIn     atomic.Int64
	usedOut    atomic.Int64
	costMicro  atomic.Int64 // cost in micro-dollars (1e-6 USD) for atomic ops
}

// NewTokenGuard creates a token guard with the given limits.
// Zero values mean unlimited.
func NewTokenGuard(maxTokens int, maxCost float64) *TokenGuard {
	return &TokenGuard{
		maxTokens: int64(maxTokens),
		maxCost:   maxCost,
	}
}

// Add records token usage and cost. Returns the new totals.
func (tg *TokenGuard) Add(inputTokens, outputTokens int, costPerInputM, costPerOutputM float64) {
	total := int64(inputTokens + outputTokens)
	tg.usedTokens.Add(total)
	tg.usedIn.Add(int64(inputTokens))
	tg.usedOut.Add(int64(outputTokens))

	// Calculate cost in micro-dollars: (tokens / 1M) * price_per_M * 1e6
	costMicro := int64(float64(inputTokens)*costPerInputM + float64(outputTokens)*costPerOutputM)
	if costMicro > 0 {
		tg.costMicro.Add(costMicro)
	}
}

// Check returns an error if any budget is exceeded.
func (tg *TokenGuard) Check() error {
	if tg.maxTokens > 0 && tg.usedTokens.Load() >= tg.maxTokens {
		return &BudgetExceededError{
			GuardName: tg.Name(),
			Message:   fmt.Sprintf("token budget exceeded: %d / %d", tg.usedTokens.Load(), tg.maxTokens),
		}
	}
	if tg.maxCost > 0 && tg.Cost() >= tg.maxCost {
		return &BudgetExceededError{
			GuardName: tg.Name(),
			Message:   fmt.Sprintf("cost budget exceeded: $%.4f / $%.4f", tg.Cost(), tg.maxCost),
		}
	}
	return nil
}

// Name returns "token_guard".
func (tg *TokenGuard) Name() string {
	return "token_guard"
}

// Usage returns total tokens used (in, out, total).
func (tg *TokenGuard) Usage() (in, out, total int) {
	return int(tg.usedIn.Load()), int(tg.usedOut.Load()), int(tg.usedTokens.Load())
}

// Cost returns accumulated cost in USD.
func (tg *TokenGuard) Cost() float64 {
	return float64(tg.costMicro.Load()) / 1e6
}

// Remaining returns remaining tokens (0 if unlimited).
func (tg *TokenGuard) Remaining() int {
	if tg.maxTokens <= 0 {
		return 0
	}
	rem := tg.maxTokens - tg.usedTokens.Load()
	if rem < 0 {
		return 0
	}
	return int(rem)
}

// MaxTokens returns the configured token limit.
func (tg *TokenGuard) MaxTokens() int {
	return int(tg.maxTokens)
}

// MaxCost returns the configured cost limit.
func (tg *TokenGuard) MaxCost() float64 {
	return tg.maxCost
}
