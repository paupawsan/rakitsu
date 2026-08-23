package llm

// StatusCoder is optionally implemented by an error returned from a
// provider's Generate call to expose the HTTP status code the provider's
// own API reported, when one is known. Retry logic (internal/agent/retry.go)
// checks for this first and only falls back to matching status codes in the
// error text when a provider error doesn't implement it (e.g. a raw network
// error) — a bare status code embedded in an otherwise-unrelated part of an
// error message (a token limit, a quota count, ...) can't be told apart
// from a real HTTP status by text alone, but the provider SDK already knows
// which one it actually got.
type StatusCoder interface {
	StatusCode() int
}
