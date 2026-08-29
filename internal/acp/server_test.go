package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/paupawsan/rakitsu/internal/config"
	"github.com/paupawsan/rakitsu/internal/debug"
	"github.com/paupawsan/rakitsu/internal/telemetry"
)

// stubRunFunc returns a RunFunc that immediately returns the given result/error.
func stubRunFunc(result string, err error) RunFunc {
	return func(ctx context.Context, cfg *config.Config, eventBus *telemetry.EventBus, _ *debug.DebugController, query string, _ func([]debug.Attachable)) (string, error) {
		return result, err
	}
}

// panicRunFunc returns a RunFunc that panics instead of returning.
func panicRunFunc(msg string) RunFunc {
	return func(ctx context.Context, cfg *config.Config, eventBus *telemetry.EventBus, _ *debug.DebugController, query string, _ func([]debug.Attachable)) (string, error) {
		panic(msg)
	}
}

// eventPublishingRunFunc publishes n events on eventBus before returning.
func eventPublishingRunFunc(n int, result string) RunFunc {
	return func(ctx context.Context, cfg *config.Config, eventBus *telemetry.EventBus, _ *debug.DebugController, query string, _ func([]debug.Attachable)) (string, error) {
		for i := 0; i < n; i++ {
			eventBus.Publish(telemetry.AgentEvent{
				ID:        fmt.Sprintf("evt-%d", i),
				EventType: telemetry.EventType("test_event"),
			})
		}
		return result, nil
	}
}

// panicAfterEventsRunFunc publishes n events, then panics — used to check
// the panic-recovery path still flushes pending events in order and doesn't
// leak the event-drain goroutine.
func panicAfterEventsRunFunc(n int, msg string) RunFunc {
	return func(ctx context.Context, cfg *config.Config, eventBus *telemetry.EventBus, _ *debug.DebugController, query string, _ func([]debug.Attachable)) (string, error) {
		for i := 0; i < n; i++ {
			eventBus.Publish(telemetry.AgentEvent{
				ID:        fmt.Sprintf("evt-%d", i),
				EventType: telemetry.EventType("test_event"),
			})
		}
		panic(msg)
	}
}

// testServer wraps an ACP server with pipe I/O for testing.
// Output is continuously drained into a channel to prevent write deadlocks.
type testServer struct {
	inW   *io.PipeWriter
	lines chan string
}

// newTestServer creates a running ACP server backed by in-memory pipes.
// The returned CancelFunc stops it. All output is drained into ts.lines.
func newTestServer(runFunc RunFunc) (*testServer, context.CancelFunc) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	srv := NewServerWithIO(runFunc, inR, outW)
	ctx, cancel := context.WithCancel(context.Background())

	ts := &testServer{
		inW:   inW,
		lines: make(chan string, 256),
	}

	go srv.Run(ctx, nil, nil) //nolint:errcheck

	// Continuously drain output so server writes never block.
	go func() {
		scanner := bufio.NewScanner(outR)
		scanner.Buffer(make([]byte, 1<<20), 1<<20)
		for scanner.Scan() {
			ts.lines <- scanner.Text()
		}
		close(ts.lines)
	}()

	return ts, cancel
}

// send writes a JSON-RPC line to the server's stdin.
func (ts *testServer) send(t *testing.T, v interface{}) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	b = append(b, '\n')
	if _, err := ts.inW.Write(b); err != nil {
		t.Fatalf("write to server: %v", err)
	}
}

// recv reads the next output line from the server within timeout.
func (ts *testServer) recv(t *testing.T, timeout time.Duration) string {
	t.Helper()
	select {
	case line, ok := <-ts.lines:
		if !ok {
			t.Fatal("server output channel closed")
		}
		return line
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for server output after %s", timeout)
		return ""
	}
}

// recvResponse reads the next response (with ID) from the server.
func (ts *testServer) recvResponse(t *testing.T, timeout time.Duration) acpResponse {
	t.Helper()
	line := ts.recv(t, timeout)
	var resp acpResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("unmarshal response: %v (line=%q)", err, line)
	}
	return resp
}

// recvNotification reads the next notification (no ID) from the server.
func (ts *testServer) recvNotification(t *testing.T, timeout time.Duration) acpNotification {
	t.Helper()
	line := ts.recv(t, timeout)
	var notif acpNotification
	if err := json.Unmarshal([]byte(line), &notif); err != nil {
		t.Fatalf("unmarshal notification: %v (line=%q)", err, line)
	}
	return notif
}

// ─── TestACP_Initialize ───────────────────────────────────────────────────────

func TestACP_Initialize(t *testing.T) {
	ts, cancel := newTestServer(stubRunFunc("", nil))
	defer cancel()

	id := json.RawMessage("1")
	ts.send(t, acpRequest{JSONRPC: "2.0", ID: id, Method: "initialize"})

	resp := ts.recvResponse(t, 2*time.Second)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("expected result, got nil")
	}

	b, _ := json.Marshal(resp.Result)
	var result initializeResult
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ProtocolVersion != "0.1" {
		t.Errorf("want protocol_version=0.1, got %q", result.ProtocolVersion)
	}
	if result.ServerInfo.Name != "rakitsu" {
		t.Errorf("want server name rakitsu, got %q", result.ServerInfo.Name)
	}
	if !result.Capabilities.Streaming {
		t.Error("expected streaming capability")
	}
	if !result.Capabilities.Cancel {
		t.Error("expected cancel capability")
	}
}

// ─── TestACP_RunComplete ──────────────────────────────────────────────────────

func TestACP_RunComplete(t *testing.T) {
	ts, cancel := newTestServer(stubRunFunc("Hello from agent", nil))
	defer cancel()

	id := json.RawMessage("2")
	ts.send(t, acpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "agent/run",
		Params:  json.RawMessage(`{"config_path":"testdata/agent.yaml","query":"hello"}`),
	})

	// First response: session_id (immediate).
	resp := ts.recvResponse(t, 2*time.Second)
	if resp.Error != nil {
		t.Fatalf("run error: %+v", resp.Error)
	}
	b, _ := json.Marshal(resp.Result)
	var runRes runResult
	json.Unmarshal(b, &runRes) //nolint:errcheck
	if runRes.SessionID == "" {
		t.Fatal("expected non-empty session_id")
	}

	// Drain notifications until agent/complete.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		notif := ts.recvNotification(t, 3*time.Second)
		if notif.Method == "agent/complete" {
			b, _ := json.Marshal(notif.Params)
			var cp completeNotificationParams
			json.Unmarshal(b, &cp) //nolint:errcheck
			if cp.SessionID != runRes.SessionID {
				t.Errorf("session_id mismatch: want %q, got %q", runRes.SessionID, cp.SessionID)
			}
			if cp.Result != "Hello from agent" {
				t.Errorf("want result %q, got %q", "Hello from agent", cp.Result)
			}
			if cp.Error != "" {
				t.Errorf("unexpected error in complete: %q", cp.Error)
			}
			return
		}
		// agent/event — keep draining
	}
	t.Fatal("never received agent/complete notification")
}

// ─── TestACP_InvalidJSON ──────────────────────────────────────────────────────

func TestACP_InvalidJSON(t *testing.T) {
	ts, cancel := newTestServer(stubRunFunc("", nil))
	defer cancel()

	// Send garbage then a valid request — server must survive.
	ts.inW.Write([]byte("not json\n"))  //nolint:errcheck
	ts.inW.Write([]byte("{bad json\n")) //nolint:errcheck
	id := json.RawMessage("1")
	ts.send(t, acpRequest{JSONRPC: "2.0", ID: id, Method: "initialize"})

	// Drain until we see a successful initialize response.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp := ts.recvResponse(t, 2*time.Second)
		if resp.Error == nil {
			return // initialize succeeded — server survived bad input
		}
		// parse error or method not found — keep reading
		if resp.Error.Code != -32700 && resp.Error.Code != -32601 {
			t.Errorf("unexpected error code: %d", resp.Error.Code)
		}
	}
	t.Fatal("never received successful initialize response after bad input")
}

// ─── TestACP_UnknownMethod ────────────────────────────────────────────────────

func TestACP_UnknownMethod(t *testing.T) {
	ts, cancel := newTestServer(stubRunFunc("", nil))
	defer cancel()

	id := json.RawMessage("1")
	ts.send(t, acpRequest{JSONRPC: "2.0", ID: id, Method: "no_such_method"})

	resp := ts.recvResponse(t, 2*time.Second)
	if resp.Error == nil {
		t.Fatal("expected error response for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("want code -32601, got %d", resp.Error.Code)
	}
	if !strings.Contains(resp.Error.Message, "method not found") {
		t.Errorf("unexpected error message: %q", resp.Error.Message)
	}
}

// ─── TestACP_RunMissingParams ─────────────────────────────────────────────────

func TestACP_RunMissingParams(t *testing.T) {
	ts, cancel := newTestServer(stubRunFunc("", nil))
	defer cancel()

	id := json.RawMessage("1")
	ts.send(t, acpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "agent/run",
		Params:  json.RawMessage(`{"query":"no config path"}`),
	})

	resp := ts.recvResponse(t, 2*time.Second)
	if resp.Error == nil {
		t.Fatal("expected error for missing config_path")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("want code -32602, got %d", resp.Error.Code)
	}
}

// ─── TestACP_Status_NotFound ──────────────────────────────────────────────────

func TestACP_Status_NotFound(t *testing.T) {
	ts, cancel := newTestServer(stubRunFunc("", nil))
	defer cancel()

	id := json.RawMessage("1")
	ts.send(t, acpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "agent/status",
		Params:  json.RawMessage(`{"session_id":"nonexistent"}`),
	})

	resp := ts.recvResponse(t, 2*time.Second)
	if resp.Error == nil {
		t.Fatal("expected error for unknown session")
	}
	if resp.Error.Code != -32001 {
		t.Errorf("want code -32001, got %d", resp.Error.Code)
	}
}

// ─── TestACP_Cancel_NotFound ──────────────────────────────────────────────────

func TestACP_Cancel_NotFound(t *testing.T) {
	ts, cancel := newTestServer(stubRunFunc("", nil))
	defer cancel()

	id := json.RawMessage("1")
	ts.send(t, acpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "agent/cancel",
		Params:  json.RawMessage(`{"session_id":"nonexistent"}`),
	})

	resp := ts.recvResponse(t, 2*time.Second)
	if resp.Error == nil {
		t.Fatal("expected error for unknown session")
	}
	if resp.Error.Code != -32001 {
		t.Errorf("want code -32001, got %d", resp.Error.Code)
	}
}

// ─── TestACP_RunFunc_PanicDoesNotCrashServer ──────────────────────────────────

func TestACP_RunFunc_PanicDoesNotCrashServer(t *testing.T) {
	ts, cancel := newTestServer(panicRunFunc("boom"))
	defer cancel()

	id := json.RawMessage("1")
	ts.send(t, acpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "agent/run",
		Params:  json.RawMessage(`{"config_path":"testdata/agent.yaml","query":"hello"}`),
	})

	resp := ts.recvResponse(t, 2*time.Second)
	if resp.Error != nil {
		t.Fatalf("run error: %+v", resp.Error)
	}
	b, _ := json.Marshal(resp.Result)
	var runRes runResult
	json.Unmarshal(b, &runRes) //nolint:errcheck

	deadline := time.Now().Add(3 * time.Second)
	var gotComplete bool
	for time.Now().Before(deadline) {
		notif := ts.recvNotification(t, 3*time.Second)
		if notif.Method == "agent/complete" {
			b, _ := json.Marshal(notif.Params)
			var cp completeNotificationParams
			json.Unmarshal(b, &cp) //nolint:errcheck
			if cp.SessionID != runRes.SessionID {
				t.Errorf("session_id mismatch: want %q, got %q", runRes.SessionID, cp.SessionID)
			}
			if !strings.Contains(cp.Error, "agent panic") {
				t.Errorf("want a panic-flavored error, got %q", cp.Error)
			}
			gotComplete = true
			break
		}
	}
	if !gotComplete {
		t.Fatal("never received agent/complete after the panic")
	}

	// The real point of this test: the server process itself must still be
	// alive and answering other requests, not just this one session.
	id2 := json.RawMessage("2")
	ts.send(t, acpRequest{JSONRPC: "2.0", ID: id2, Method: "initialize"})
	initResp := ts.recvResponse(t, 2*time.Second)
	if initResp.Error != nil {
		t.Fatalf("server did not survive the panic: %+v", initResp.Error)
	}
}

// ─── TestACP_OversizedLine_DoesNotKillServer ──────────────────────────────────

func TestACP_OversizedLine_DoesNotKillServer(t *testing.T) {
	ts, cancel := newTestServer(stubRunFunc("", nil))
	defer cancel()

	// Bigger than the old fixed bufio.Scanner buffer (1<<20 bytes) this
	// replaces — that scanner died permanently on a line over its buffer
	// size (bufio.ErrTooLong), silently dropping every request afterward.
	huge := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":"` + strings.Repeat("x", 2<<20) + `"}` + "\n"
	if _, err := ts.inW.Write([]byte(huge)); err != nil {
		t.Fatalf("write huge line: %v", err)
	}

	resp := ts.recvResponse(t, 3*time.Second)
	if resp.Error != nil {
		t.Fatalf("oversized-but-valid line should still be processed, got error: %+v", resp.Error)
	}

	// Confirm the server is still alive for subsequent requests too.
	id := json.RawMessage("2")
	ts.send(t, acpRequest{JSONRPC: "2.0", ID: id, Method: "initialize"})
	resp2 := ts.recvResponse(t, 2*time.Second)
	if resp2.Error != nil {
		t.Fatalf("server did not survive the oversized line: %+v", resp2.Error)
	}
}

// ─── TestACP_Run_RespectsContextCancellation ──────────────────────────────────

func TestACP_Run_RespectsContextCancellation(t *testing.T) {
	inR, inW := io.Pipe()
	defer inW.Close()
	outR, outW := io.Pipe()
	go io.Copy(io.Discard, outR) //nolint:errcheck

	srv := NewServerWithIO(stubRunFunc("", nil), inR, outW)
	ctx, cancel := context.WithCancel(context.Background())

	runReturned := make(chan error, 1)
	go func() {
		runReturned <- srv.Run(ctx, nil, nil)
	}()

	// Give Run a moment to start blocking on the read — in stays open the
	// whole time, so only ctx cancellation should make Run return.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-runReturned:
		if err != context.Canceled {
			t.Errorf("want context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx cancellation, even though in stayed open")
	}
}

// ─── TestACP_Run_WaitsForInFlightAgentRunOnEOF ────────────────────────────────

func TestACP_Run_WaitsForInFlightAgentRunOnEOF(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	slowRunFunc := RunFunc(func(ctx context.Context, cfg *config.Config, eventBus *telemetry.EventBus, _ *debug.DebugController, query string, _ func([]debug.Attachable)) (string, error) {
		close(started)
		<-release
		return "done", nil
	})

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	go io.Copy(io.Discard, outR) //nolint:errcheck

	srv := NewServerWithIO(slowRunFunc, inR, outW)

	runReturned := make(chan error, 1)
	go func() {
		runReturned <- srv.Run(context.Background(), nil, nil)
	}()

	id := json.RawMessage("1")
	req := acpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "agent/run",
		Params:  json.RawMessage(`{"config_path":"testdata/agent.yaml","query":"hello"}`),
	}
	b, _ := json.Marshal(req)
	if _, err := inW.Write(append(b, '\n')); err != nil {
		t.Fatalf("write request: %v", err)
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("agent/run never started")
	}

	// EOF while the agent/run goroutine is still blocked inside runFunc.
	inW.Close()

	select {
	case err := <-runReturned:
		t.Fatalf("Run returned (%v) before the in-flight agent/run goroutine finished", err)
	case <-time.After(200 * time.Millisecond):
		// Good — Run is still waiting on the in-flight goroutine.
	}

	close(release)

	select {
	case <-runReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the in-flight agent/run goroutine finished")
	}
}

// ─── TestACP_StringID_RoundTrips ───────────────────────────────────────────────

func TestACP_StringID_RoundTrips(t *testing.T) {
	ts, cancel := newTestServer(stubRunFunc("", nil))
	defer cancel()

	ts.send(t, json.RawMessage(`{"jsonrpc":"2.0","id":"abc-123","method":"initialize"}`))

	resp := ts.recvResponse(t, 2*time.Second)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if string(resp.ID) != `"abc-123"` {
		t.Errorf("want id %q, got %q", `"abc-123"`, string(resp.ID))
	}
}

// ─── TestACP_Status_FindsSessionRightAfterComplete ────────────────────────────

func TestACP_Status_FindsSessionRightAfterComplete(t *testing.T) {
	ts, cancel := newTestServer(stubRunFunc("done", nil))
	defer cancel()

	id := json.RawMessage("1")
	ts.send(t, acpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "agent/run",
		Params:  json.RawMessage(`{"config_path":"testdata/agent.yaml","query":"hello"}`),
	})

	resp := ts.recvResponse(t, 2*time.Second)
	b, _ := json.Marshal(resp.Result)
	var runRes runResult
	json.Unmarshal(b, &runRes) //nolint:errcheck

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		notif := ts.recvNotification(t, 3*time.Second)
		if notif.Method == "agent/complete" {
			break
		}
	}

	// Immediately poll status — the grace period must still find it.
	statusID := json.RawMessage("2")
	ts.send(t, acpRequest{
		JSONRPC: "2.0",
		ID:      statusID,
		Method:  "agent/status",
		Params:  json.RawMessage(fmt.Sprintf(`{"session_id":%q}`, runRes.SessionID)),
	})
	statusResp := ts.recvResponse(t, 2*time.Second)
	if statusResp.Error != nil {
		t.Fatalf("expected the session to still be found right after agent/complete, got error: %+v", statusResp.Error)
	}
}

// ─── TestACP_EventOrdering_CompleteArrivesAfterAllEvents ──────────────────────

func TestACP_EventOrdering_CompleteArrivesAfterAllEvents(t *testing.T) {
	const numEvents = 5
	ts, cancel := newTestServer(eventPublishingRunFunc(numEvents, "done"))
	defer cancel()

	id := json.RawMessage("1")
	ts.send(t, acpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "agent/run",
		Params:  json.RawMessage(`{"config_path":"testdata/agent.yaml","query":"hello"}`),
	})

	resp := ts.recvResponse(t, 2*time.Second)
	if resp.Error != nil {
		t.Fatalf("run error: %+v", resp.Error)
	}

	seenEvents := 0
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		notif := ts.recvNotification(t, 3*time.Second)
		switch notif.Method {
		case "agent/event":
			seenEvents++
		case "agent/complete":
			if seenEvents != numEvents {
				t.Errorf("agent/complete arrived after only %d/%d agent/event notifications", seenEvents, numEvents)
			}
			return
		}
	}
	t.Fatal("never received agent/complete")
}

// ─── TestACP_Notification_GetsNoResponse ──────────────────────────────────────

func TestACP_Notification_GetsNoResponse(t *testing.T) {
	ts, cancel := newTestServer(stubRunFunc("", nil))
	defer cancel()

	// A JSON-RPC 2.0 Notification (no "id" member at all) must never
	// receive a response — this would normally produce a "session not
	// found" error response if it were treated as a request.
	ts.send(t, json.RawMessage(`{"jsonrpc":"2.0","method":"agent/status","params":{"session_id":"nonexistent"}}`))

	select {
	case line, ok := <-ts.lines:
		if ok {
			t.Fatalf("expected no response to a notification, got: %s", line)
		}
	case <-time.After(300 * time.Millisecond):
		// No output arrived — correct.
	}

	// The server must still be alive and processing real requests.
	id := json.RawMessage("1")
	ts.send(t, acpRequest{JSONRPC: "2.0", ID: id, Method: "initialize"})
	resp := ts.recvResponse(t, 2*time.Second)
	if resp.Error != nil {
		t.Fatalf("server did not survive the notification: %+v", resp.Error)
	}
}

// ─── TestACP_ExplicitNullID_StillRespondsWithNull ─────────────────────────────

func TestACP_ExplicitNullID_StillRespondsWithNull(t *testing.T) {
	ts, cancel := newTestServer(stubRunFunc("", nil))
	defer cancel()

	// An explicit `"id": null` is a real request, not a Notification — the
	// server must still respond, with id: null, per the JSON-RPC 2.0 spec.
	ts.send(t, json.RawMessage(`{"jsonrpc":"2.0","id":null,"method":"initialize"}`))

	resp := ts.recvResponse(t, 2*time.Second)
	if string(resp.ID) != "null" {
		t.Errorf("want id \"null\", got %q", string(resp.ID))
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %+v", resp.Error)
	}
}

// ─── TestACP_Run_NoWaitGroupRaceOnConcurrentCancelAndDispatch ─────────────────

// TestACP_Run_NoWaitGroupRaceOnConcurrentCancelAndDispatch stresses the exact
// interleaving the round-2 review flagged: ctx cancellation racing the
// reader goroutine's own s.wg.Add(1) calls. It doesn't assert much on its
// own — the point is `go test -race` catching a concurrent Add/Wait if the
// shutdownMu guard in Run ever regresses.
func TestACP_Run_NoWaitGroupRaceOnConcurrentCancelAndDispatch(t *testing.T) {
	inR, inW := io.Pipe()
	defer inW.Close()
	outR, outW := io.Pipe()
	go io.Copy(io.Discard, outR) //nolint:errcheck

	srv := NewServerWithIO(stubRunFunc("ok", nil), inR, outW)
	ctx, cancel := context.WithCancel(context.Background())

	runReturned := make(chan error, 1)
	go func() {
		runReturned <- srv.Run(ctx, nil, nil)
	}()

	stop := make(chan struct{})
	go func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			req := acpRequest{JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", i)), Method: "initialize"}
			b, _ := json.Marshal(req)
			if _, err := inW.Write(append(b, '\n')); err != nil {
				return
			}
		}
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	close(stop)

	select {
	case <-runReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx cancellation under concurrent dispatch")
	}
}

// ─── TestACP_Panic_FlushesEventsAndDoesNotLeakDrainGoroutine ──────────────────

// Regression: the panic-recovery path in handleRun wrote agent/complete
// directly, without unsubscribing from the event bus or waiting for the
// drain goroutine first. That broke two things at once: agent/complete could
// arrive before pending agent/event notifications were flushed, and — since
// eventCh was never closed — the drain goroutine's `for event := range
// eventCh` blocked forever, leaking one goroutine (and its EventBus) per
// panicking run for the life of the process.
//
// Calls handleRun directly (bypassing Run's reader loop and newTestServer's
// own pipe/scanner goroutines entirely) so the only goroutines in play are
// the ones handleRun itself spawns — otherwise the harness's own
// long-lived plumbing swamps the specific leak this test exists to catch.
func TestACP_Panic_FlushesEventsAndDoesNotLeakDrainGoroutine(t *testing.T) {
	const numEvents = 5
	const numRuns = 20

	runtime.GC()
	before := runtime.NumGoroutine()

	for i := 0; i < numRuns; i++ {
		var buf syncBuffer
		s := NewServerWithIO(panicAfterEventsRunFunc(numEvents, "boom"), nil, &buf)
		req := acpRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage("1"),
			Method:  "agent/run",
			Params:  json.RawMessage(`{"config_path":"testdata/agent.yaml","query":"hello"}`),
		}
		s.handleRun(context.Background(), req)
		s.wg.Wait() // blocks until the background goroutine's Done() defer runs — i.e. everything, including cleanupEvents, has finished

		seenEvents := 0
		gotComplete := false
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			var notif acpNotification
			if err := json.Unmarshal([]byte(line), &notif); err != nil {
				continue // the immediate runResult response, not a notification
			}
			switch notif.Method {
			case "agent/event":
				seenEvents++
			case "agent/complete":
				if seenEvents != numEvents {
					t.Errorf("run %d: agent/complete arrived after only %d/%d agent/event notifications", i, seenEvents, numEvents)
				}
				gotComplete = true
			}
		}
		if !gotComplete {
			t.Fatalf("run %d: no agent/complete notification found in output: %s", i, buf.String())
		}
	}

	// s.wg.Wait() above already guarantees the drain goroutine saw eventCh
	// close and its range loop returned before close(drainDone) — the only
	// remaining slack is the scheduler actually retiring the now-finished
	// goroutine before NumGoroutine's next snapshot.
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()
	if after > before+5 {
		t.Errorf("goroutine count grew from %d to %d after %d panicking runs — the event-drain goroutine may be leaking", before, after, numRuns)
	}
}

// syncBuffer is a bytes.Buffer safe for the concurrent writes writeLine
// makes under s.mu — reads are only ever done here after s.wg.Wait() has
// returned, once every writer goroutine has already finished.
type syncBuffer struct {
	bytes.Buffer
}
