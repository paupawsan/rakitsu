package chat

import (
	"context"
	"sync"
	"time"
)

// InFlight tracks directed-chat sends running off the update loop, so process
// shutdown can cancel them and wait for them to unwind before the tool
// registries they hold are torn down.
//
// This exists because bubbletea never waits on Cmd goroutines: handleCommands
// runs each Cmd detached and shutdown waits only on its message handlers and
// the input reader. p.Run() therefore returns with a Roster.Send still
// executing, and the caller's cleanup then closes the same global tool
// instances (MCP subprocesses included) that the live side-thread agent is
// holding. The quit guard closes most of that window; this closes the rest,
// including the one Ctrl+C opens by clearing `generating` the instant it
// cancels, while the cancelled Send is still unwinding its HTTP call.
//
// A nil *InFlight is usable and tracks nothing, so unit tests and callers
// that never start a side chat need no wiring.
type InFlight struct {
	mu      sync.Mutex
	wg      sync.WaitGroup
	next    int64
	cancels map[int64]context.CancelFunc
}

// Begin registers a send and returns its context, that send's own cancel (for
// Ctrl+C, which cancels one thread rather than all of them), and a done
// callback the send must call exactly once when it returns.
func (f *InFlight) Begin(parent context.Context) (context.Context, context.CancelFunc, func()) {
	ctx, cancel := context.WithCancel(parent)
	if f == nil {
		return ctx, cancel, func() {}
	}
	f.mu.Lock()
	if f.cancels == nil {
		f.cancels = make(map[int64]context.CancelFunc)
	}
	f.next++
	id := f.next
	f.cancels[id] = cancel
	f.wg.Add(1)
	f.mu.Unlock()

	var once sync.Once
	return ctx, cancel, func() {
		once.Do(func() {
			f.mu.Lock()
			delete(f.cancels, id)
			f.mu.Unlock()
			f.wg.Done()
		})
	}
}

// CancelAll cancels every send currently in flight. Already-cancelled sends
// are unaffected.
func (f *InFlight) CancelAll() {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cancel := range f.cancels {
		cancel()
	}
}

// Wait blocks until every registered send has returned, or until d elapses.
// Reports whether they all finished. The bound is the point: a send that
// ignores its cancellation must not hang the terminal on exit, so the caller
// gets an answer either way and can say what happened. Call only after no
// further Begin can occur.
func (f *InFlight) Wait(d time.Duration) bool {
	if f == nil {
		return true
	}
	done := make(chan struct{})
	go func() {
		f.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}
