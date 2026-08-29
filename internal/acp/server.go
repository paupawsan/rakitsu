package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/paupawsan/rakitsu/internal/config"
	"github.com/paupawsan/rakitsu/internal/debug"
	"github.com/paupawsan/rakitsu/internal/telemetry"
)

// RunFunc is the signature of executeConfig from cmd/rakitsu/run.go.
// The ACP server calls this to run an agent pipeline.
// The registrar callback (if non-nil) registers agents for mid-run debug attach.
type RunFunc func(
	ctx context.Context,
	cfg *config.Config,
	eventBus *telemetry.EventBus,
	debugCtrl *debug.DebugController,
	query string,
	registrar func([]debug.Attachable),
) (string, error)

// ─── JSON-RPC types ───────────────────────────────────────────────────────────

// acpRequest.ID and acpResponse.ID are json.RawMessage, not *int: JSON-RPC 2.0
// permits a request id to be a string, a number, or null. A client using
// string ids (common in JSON-RPC client libraries) would otherwise fail
// json.Unmarshal entirely for that line, routing a well-formed request into
// the "malformed JSON" branch and losing the id needed to correlate the
// error. json.RawMessage round-trips whatever shape the client sent.
type acpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type acpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *acpError       `json:"error,omitempty"`
}

type acpNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type acpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ─── Server ───────────────────────────────────────────────────────────────────

// sessionGracePeriod is how long a completed/failed/cancelled session stays
// in the registry before being removed. Without it, the deferred delete in
// handleRun's run goroutine races an agent/status poll that a client sends
// right after observing the agent/complete notification — the two happen
// microseconds apart, and the poll loses that race far more often than not.
const sessionGracePeriod = 30 * time.Second

// Server is the ACP stdio server. It reads JSON-RPC requests from in,
// executes agent pipelines via runFunc, and writes JSON-RPC responses/
// notifications to out.
type Server struct {
	runFunc  RunFunc
	in       io.Reader
	out      io.Writer
	mu       sync.Mutex // protects writes to out
	sessions *sessionMap
	wg       sync.WaitGroup // tracks all in-flight dispatch + agent/run work

	shutdownMu sync.Mutex // guards stopped, serializing it against wg.Add in the reader loop
	stopped    bool       // set before wg.Wait(); see Run's comment
}

// NewServer creates an ACP server that reads from os.Stdin and writes to os.Stdout.
// Use NewServerWithIO for testing.
func NewServer(runFunc RunFunc) *Server {
	return &Server{
		runFunc:  runFunc,
		sessions: newSessionMap(),
	}
}

// NewServerWithIO creates an ACP server with injected I/O (for testing).
func NewServerWithIO(runFunc RunFunc, in io.Reader, out io.Writer) *Server {
	return &Server{
		runFunc:  runFunc,
		in:       in,
		out:      out,
		sessions: newSessionMap(),
	}
}

// Run blocks, reading newline-delimited JSON-RPC requests until in is closed
// or ctx is cancelled. It returns only once every request dispatched before
// shutdown — including the background agent/run goroutines that outlive
// their initial dispatch — has actually finished, so a caller can safely
// treat Run returning as "safe to shut down." No new request is dispatched
// once shutdown begins.
func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	if s.in == nil {
		s.in = in
	}
	if s.out == nil {
		s.out = out
	}

	// The read loop runs on its own goroutine so Run can select on ctx.Done()
	// instead of blocking forever on a synchronous read when in stays open.
	readDone := make(chan error, 1)
	go func() {
		// bufio.Reader.ReadString has no fixed per-line size ceiling, unlike
		// the bufio.Scanner + fixed buffer this replaces: a single line over
		// that buffer's size (plausible for agent/run's query field, which
		// could carry a pasted diff) made Scan() return false permanently
		// with bufio.ErrTooLong — Scanner cannot recover after that, so the
		// loop exited and Run returned silently, dropping every subsequent
		// request for the life of the process.
		reader := bufio.NewReader(s.in)
		for {
			line, err := reader.ReadString('\n')
			trimmed := bytesTrimNewline(line)
			if len(trimmed) > 0 {
				var req acpRequest
				if jsonErr := json.Unmarshal([]byte(trimmed), &req); jsonErr != nil {
					// Malformed JSON — parse errors are reported with an
					// explicit JSON "null" id per the JSON-RPC 2.0 spec, not
					// suppressed the way a true notification's absent id is
					// (see writeResponse/writeError).
					s.writeError(json.RawMessage("null"), -32700, "parse error")
				} else {
					// s.wg.Add must never happen concurrently with or after
					// s.wg.Wait() below — sync.WaitGroup's own docs call that
					// undefined behavior. shutdownMu serializes this check
					// against Run setting s.stopped before it calls Wait, so
					// either this Add is guaranteed to happen before that
					// Wait, or stopped is already true and this request is
					// dropped instead of dispatched.
					s.shutdownMu.Lock()
					if s.stopped {
						s.shutdownMu.Unlock()
					} else {
						s.wg.Add(1)
						s.shutdownMu.Unlock()
						go func() {
							defer s.wg.Done()
							s.dispatch(ctx, req)
						}()
					}
				}
			}
			if err != nil {
				if err == io.EOF {
					readDone <- nil
				} else {
					readDone <- err
				}
				return
			}
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		runErr = ctx.Err()
	case runErr = <-readDone:
	}

	// Stop accepting new dispatches before waiting — see the Add-site
	// comment above for why this ordering is what makes s.wg.Wait() safe.
	// The reader goroutine itself may still be blocked on the next
	// reader.ReadString call after Run returns (nothing closes s.in here);
	// that's fine, since it can no longer dispatch anything once stopped.
	s.shutdownMu.Lock()
	s.stopped = true
	s.shutdownMu.Unlock()

	// Wait for every request dispatched before shutdown (including
	// agent/run's background goroutine, which registers its own
	// s.wg.Add(1) — see handleRun) to actually finish before returning.
	s.wg.Wait()
	return runErr
}

// bytesTrimNewline strips a trailing "\n" and, before that, "\r" — matching
// bufio.Scanner's ScanLines behavior for CRLF input — without allocating a
// new string via strings.TrimRight for the common case.
func bytesTrimNewline(line string) string {
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line
}

// ─── Dispatch ─────────────────────────────────────────────────────────────────

func (s *Server) dispatch(ctx context.Context, req acpRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "agent/run":
		s.handleRun(ctx, req)
	case "agent/cancel":
		s.handleCancel(req)
	case "agent/status":
		s.handleStatus(req)
	default:
		s.writeError(req.ID, -32601, fmt.Sprintf("method not found: %q", req.Method))
	}
}

// ─── initialize ───────────────────────────────────────────────────────────────

type initializeResult struct {
	ProtocolVersion string     `json:"protocol_version"`
	ServerInfo      serverInfo `json:"server_info"`
	Capabilities    caps       `json:"capabilities"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type caps struct {
	Streaming bool `json:"streaming"`
	Cancel    bool `json:"cancel"`
	Status    bool `json:"status"`
}

func (s *Server) handleInitialize(req acpRequest) {
	s.writeResponse(req.ID, initializeResult{
		ProtocolVersion: "0.1",
		ServerInfo:      serverInfo{Name: "rakitsu", Version: "0.1.0"},
		Capabilities:    caps{Streaming: true, Cancel: true, Status: true},
	})
}

// ─── agent/run ────────────────────────────────────────────────────────────────

type runParams struct {
	ConfigPath string `json:"config_path"`
	Query      string `json:"query"`
	TimeoutSec int    `json:"timeout_sec"`
}

type runResult struct {
	SessionID string `json:"session_id"`
}

type eventNotificationParams struct {
	SessionID string               `json:"session_id"`
	Event     telemetry.AgentEvent `json:"event"`
}

type completeNotificationParams struct {
	SessionID string `json:"session_id"`
	Result    string `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleRun(ctx context.Context, req acpRequest) {
	var params runParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(req.ID, -32602, "invalid params: "+err.Error())
		return
	}
	if params.ConfigPath == "" {
		s.writeError(req.ID, -32602, "config_path is required")
		return
	}
	if params.Query == "" {
		s.writeError(req.ID, -32602, "query is required")
		return
	}

	cfg, err := config.Load(params.ConfigPath)
	if err != nil {
		s.writeError(req.ID, -32603, "load config: "+err.Error())
		return
	}

	timeoutSec := params.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 300
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	sessionID := uuid.New().String()
	sess := newSession(sessionID, cancel)
	s.sessions.add(sess)

	// Respond immediately with the session ID before execution starts.
	s.writeResponse(req.ID, runResult{SessionID: sessionID})

	// Run the agent pipeline in a goroutine, streaming events as notifications.
	// Registered on s.wg so Run() doesn't return while this is still in
	// flight — see Run's comment on why the outer dispatch goroutine alone
	// isn't enough to track this. Unlike the reader loop's Add, this one
	// doesn't need its own shutdownMu check: it only runs from inside the
	// dispatch goroutine the reader loop already registered, so the
	// WaitGroup counter is guaranteed above zero for the whole duration of
	// this call — the Add-after-Wait race that guard exists for can't happen
	// here. That invariant depends on dispatch always being reached through
	// the reader loop's guarded Add, so keep it that way if this ever changes.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		// A grace period, not an immediate delete: a client polling
		// agent/status right after observing agent/complete would otherwise
		// race this delete by microseconds and almost always lose.
		defer time.AfterFunc(sessionGracePeriod, func() { s.sessions.delete(sessionID) })

		eventBus := telemetry.NewEventBus(1024)
		eventCh := eventBus.Subscribe()

		// Drain events → agent/event notifications. drainDone signals once
		// the range loop has finished flushing everything Unsubscribe's
		// channel-close left buffered, so agent/complete is only written
		// after the client has already seen every agent/event that causally
		// preceded it — on BOTH the normal and panic paths below.
		drainDone := make(chan struct{})
		go func() {
			defer close(drainDone)
			for event := range eventCh {
				s.writeNotification("agent/event", eventNotificationParams{
					SessionID: sessionID,
					Event:     event,
				})
			}
		}()
		var cleanupEventsOnce sync.Once
		cleanupEvents := func() {
			cleanupEventsOnce.Do(func() {
				eventBus.Unsubscribe(eventCh)
				<-drainDone
			})
		}

		// Any panic in s.runFunc — the full agent/orchestrator/provider/tool
		// execution stack — must not escape this goroutine: unrecovered, it
		// would crash the whole ACP process, taking down every other
		// concurrently-running session on the same stdio connection.
		// internal/server/runner.go guards the same RunFunc call point the
		// same way. cleanupEvents is called explicitly here rather than left
		// to a defer at this call site: a plain defer registered before
		// s.runFunc would run AFTER this recover defer during panic unwind
		// (defers unwind LIFO — this one, registered first, runs last), so
		// agent/complete below would be written before the drain finished
		// flushing every preceding agent/event, breaking the very ordering
		// this cleanup exists to guarantee. Calling it explicitly, exactly
		// where the cleanup needs to happen relative to each notification
		// write, is what the sync.Once above is for (idempotent).
		defer func() {
			if p := recover(); p != nil {
				cleanupEvents()
				msg := fmt.Sprintf("agent panic: %v", p)
				sess.fail(msg)
				s.writeNotification("agent/complete", completeNotificationParams{
					SessionID: sessionID,
					Error:     msg,
				})
			}
		}()

		result, runErr := s.runFunc(runCtx, cfg, eventBus, nil, params.Query, nil)

		cleanupEvents()

		if runErr != nil {
			sess.fail(runErr.Error())
			s.writeNotification("agent/complete", completeNotificationParams{
				SessionID: sessionID,
				Error:     runErr.Error(),
			})
		} else {
			sess.complete(result)
			s.writeNotification("agent/complete", completeNotificationParams{
				SessionID: sessionID,
				Result:    result,
			})
		}
	}()
}

// ─── agent/cancel ─────────────────────────────────────────────────────────────

type cancelParams struct {
	SessionID string `json:"session_id"`
}

func (s *Server) handleCancel(req acpRequest) {
	var params cancelParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(req.ID, -32602, "invalid params: "+err.Error())
		return
	}
	sess, ok := s.sessions.get(params.SessionID)
	if !ok {
		s.writeError(req.ID, -32001, "session not found: "+params.SessionID)
		return
	}
	sess.cancel()
	sess.cancelled()
	s.writeResponse(req.ID, struct{}{})
}

// ─── agent/status ─────────────────────────────────────────────────────────────

type statusParams struct {
	SessionID string `json:"session_id"`
}

type statusResult struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Result    string `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleStatus(req acpRequest) {
	var params statusParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(req.ID, -32602, "invalid params: "+err.Error())
		return
	}
	sess, ok := s.sessions.get(params.SessionID)
	if !ok {
		s.writeError(req.ID, -32001, "session not found: "+params.SessionID)
		return
	}
	st, result, errMsg := sess.snapshot()
	s.writeResponse(req.ID, statusResult{
		SessionID: params.SessionID,
		Status:    string(st),
		Result:    result,
		Error:     errMsg,
	})
}

// ─── I/O helpers ──────────────────────────────────────────────────────────────

// writeResponse writes a JSON-RPC response for id, unless id is nil.
// acpRequest.ID unmarshals to nil exactly when the request's "id" member was
// absent — i.e. a JSON-RPC 2.0 Notification, which the spec forbids ever
// responding to. This is distinct from an explicit `"id": null`, which
// unmarshals to the non-nil json.RawMessage("null") and still gets a
// response, per spec.
func (s *Server) writeResponse(id json.RawMessage, result interface{}) {
	if id == nil {
		return
	}
	s.writeLine(acpResponse{JSONRPC: "2.0", ID: id, Result: result})
}

// writeError mirrors writeResponse's notification check — see its comment.
// A caller that must respond despite not having parsed an id (e.g. a JSON
// parse error) passes an explicit json.RawMessage("null") rather than nil.
func (s *Server) writeError(id json.RawMessage, code int, msg string) {
	if id == nil {
		return
	}
	s.writeLine(acpResponse{JSONRPC: "2.0", ID: id, Error: &acpError{Code: code, Message: msg}})
}

func (s *Server) writeNotification(method string, params interface{}) {
	s.writeLine(acpNotification{JSONRPC: "2.0", Method: method, Params: params})
}

func (s *Server) writeLine(v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	b = append(b, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	s.out.Write(b) //nolint:errcheck
}
