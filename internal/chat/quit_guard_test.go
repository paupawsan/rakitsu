package chat

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestEveryQuitPathHonoursTheGeneratingGuard is T6b's first half. Ctrl+C's
// guard was written to keep the TUI alive while a thread is still generating,
// precisely so an in-flight Roster.Send is never torn down mid-call — but
// Ctrl+D, /exit, /quit and /q all returned tea.Quit unconditionally and
// walked straight past it. bubbletea never waits on Cmd goroutines, so
// p.Run() returned with the Send still executing and cleanupAll then closed
// the very tool instances that Send was holding.
func TestEveryQuitPathHonoursTheGeneratingGuard(t *testing.T) {
	cases := []struct {
		name string
		quit func(Model) (tea.Model, tea.Cmd)
	}{
		{"ctrl+d", func(m Model) (tea.Model, tea.Cmd) { return m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD}) }},
		{"/exit", func(m Model) (tea.Model, tea.Cmd) { return m.handleSlashCommand(&SlashCommand{Name: "exit"}) }},
		{"/quit", func(m Model) (tea.Model, tea.Cmd) { return m.handleSlashCommand(&SlashCommand{Name: "quit"}) }},
		{"/q", func(m Model) (tea.Model, tea.Cmd) { return m.handleSlashCommand(&SlashCommand{Name: "q"}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := threadModel(t)
			tr := m.threads["Reviewer"]
			tr.generating = true
			m.threads["Reviewer"] = tr

			_, cmd := tc.quit(m)
			if isQuitCmd(cmd) {
				t.Fatalf("%s quit while a side thread was still generating", tc.name)
			}

			// ...and still quits once nothing is in flight.
			idle := threadModel(t)
			if _, cmd := tc.quit(idle); !isQuitCmd(cmd) {
				t.Errorf("%s must still quit when nothing is generating", tc.name)
			}
		})
	}
}

// TestInFlightCancelsAndWaits is T6b's second half. The guard alone is not
// enough: Ctrl+C sets generating=false the instant it cancels, while the
// cancelled Send is still unwinding its HTTP call, so a second Ctrl+C sails
// through. Shutdown therefore has to cancel what is in flight and wait for it
// — with a bound, so exit can never hang.
func TestInFlightCancelsAndWaits(t *testing.T) {
	var f InFlight
	ctx, _, done := f.Begin(context.Background())

	returned := make(chan struct{})
	go func() {
		<-ctx.Done() // unwinds only once shutdown cancels it
		done()
		close(returned)
	}()

	f.CancelAll()
	if !f.Wait(2 * time.Second) {
		t.Fatal("Wait timed out — an in-flight send that honours its context must be waited out, not abandoned")
	}
	<-returned
}

// TestInFlightWaitIsBounded: a send that ignores its cancellation must not
// hang the process on exit.
func TestInFlightWaitIsBounded(t *testing.T) {
	var f InFlight
	_, _, done := f.Begin(context.Background())
	defer done()

	f.CancelAll()
	start := time.Now()
	if f.Wait(50 * time.Millisecond) {
		t.Error("Wait reported success while a send was still running")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Wait took %v — the bound must cap how long exit can block", elapsed)
	}
}

// TestNilInFlightIsUsable keeps every unit test and non-chat caller working
// without wiring a tracker.
func TestNilInFlightIsUsable(t *testing.T) {
	var f *InFlight
	ctx, cancel, done := f.Begin(context.Background())
	if ctx == nil || cancel == nil || done == nil {
		t.Fatal("a nil InFlight must still hand back a usable context")
	}
	done()
	cancel()
	f.CancelAll()
	if !f.Wait(time.Second) {
		t.Error("a nil InFlight has nothing to wait for")
	}
}

// TestSubmitToAgentRegistersWithTheTracker pins the wiring: without it the
// tracker is an empty box and shutdown waits for nothing.
func TestSubmitToAgentRegistersWithTheTracker(t *testing.T) {
	m := threadModel(t)
	m.inFlight = &InFlight{}
	m = m.switchThread("Reviewer")

	next, cmd := m.submitToAgent("Reviewer", "hi")
	m = next.(Model)
	if m.inFlight.Wait(20 * time.Millisecond) {
		t.Error("the send must be registered before its Cmd runs, or shutdown races it")
	}
	cmd() // roster has no builder, so this returns immediately
	if !m.inFlight.Wait(2 * time.Second) {
		t.Error("a finished send must release its slot")
	}
}
