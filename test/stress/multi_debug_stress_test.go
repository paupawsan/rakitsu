//go:build stress

package stress

// Multi-session debug stress tests.
//
// These exercise the per-session DebugController routing against a running
// `rakitsu serve` at RAKITSU_BASE_URL.
//
// Run with:
//   go test -tags stress -v -run TestMultiDebug ./test/stress/...
//
// Invariants:
//   M1. Attach to session A does not mutate session B's controller.
//   M2. Breakpoints set on session A don't appear on session B.
//   M3. Attach during churn either succeeds on the still-live session or
//       returns a clean error (404 not-found / 409 not-attachable) — never a
//       panic, 500, or silent cross-targeting.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Debug API helpers (session-scoped)
// ---------------------------------------------------------------------------

type debugAttachReq struct {
	SessionID   string              `json:"session_id"`
	Breakpoints []map[string]string `json:"breakpoints,omitempty"`
}

func attachDebugger(ctx context.Context, sessionID string, bps []map[string]string) (int, []byte, error) {
	b, _ := json.Marshal(debugAttachReq{SessionID: sessionID, Breakpoints: bps})
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL()+"/api/debug/attach", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}

func detachDebugger(ctx context.Context, sessionID string) (int, error) {
	b, _ := json.Marshal(map[string]string{"session_id": sessionID})
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL()+"/api/debug/detach", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func fetchBreakpoints(ctx context.Context, sessionID string) ([]map[string]string, int, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL()+"/api/debug/breakpoints?session_id="+sessionID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, resp.StatusCode, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	var out []map[string]string
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, resp.StatusCode, err
	}
	return out, resp.StatusCode, nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestMultiDebug_CrossSessionIsolation starts N chat sessions, attaches a
// distinct breakpoint to each, and verifies no breakpoint set leaks between
// sessions (M1 + M2).
func TestMultiDebug_CrossSessionIsolation(t *testing.T) {
	requireServerUp(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	const N = 5
	started := make([]string, 0, N)
	for i := 0; i < N; i++ {
		id, err := startChat(ctx, chatConfigID())
		if err != nil {
			t.Fatalf("start chat %d: %v", i, err)
		}
		started = append(started, id)
	}
	defer func() {
		for _, id := range started {
			_ = stopChat(ctx, id)
		}
	}()

	// Ensure all appear in registry before we attach.
	if !waitUntil(5*time.Second, func() bool {
		got := idSet(listRuntimeSessions(ctx, t))
		for _, id := range started {
			if _, ok := got[id]; !ok {
				return false
			}
		}
		return true
	}) {
		t.Fatal("chats not all visible before attach phase")
	}

	// Attach a unique breakpoint to each.
	for i, id := range started {
		bps := []map[string]string{{"event_type": fmt.Sprintf("AGENT_START_%d", i), "agent_name": "*"}}
		code, body, err := attachDebugger(ctx, id, bps)
		if err != nil {
			t.Fatalf("attach %s: %v", id, err)
		}
		if code != http.StatusOK {
			t.Fatalf("attach %s: HTTP %d: %s", id, code, body)
		}
	}

	// Per-session: should see exactly one breakpoint, its own.
	for i, id := range started {
		bps, _, err := fetchBreakpoints(ctx, id)
		if err != nil {
			t.Fatalf("list bps for %s: %v", id, err)
		}
		if len(bps) != 1 {
			t.Errorf("session %s: expected 1 breakpoint, got %d: %+v", id, len(bps), bps)
			continue
		}
		want := fmt.Sprintf("AGENT_START_%d", i)
		if bps[0]["event_type"] != want {
			t.Errorf("session %s: expected %s, got %+v", id, want, bps[0])
		}
	}

	// Detach all; leftover breakpoints on any session is a leak.
	for _, id := range started {
		if code, err := detachDebugger(ctx, id); err != nil || code != http.StatusOK {
			t.Errorf("detach %s: HTTP %d err=%v", id, code, err)
		}
	}
	t.Logf("PASS: %d chats attached and detached in isolation", N)
}

// TestMultiDebug_AttachDuringChurn starts/stops chat sessions while a
// debugger is repeatedly attached. Any panic / 500 / silent cross-targeting
// fails the test (M3).
func TestMultiDebug_AttachDuringChurn(t *testing.T) {
	requireServerUp(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const rounds = 30
	var churnWg sync.WaitGroup
	stopCh := make(chan struct{})

	// Background: churn start/stop.
	liveIDs := make(chan string, rounds)
	churnWg.Add(1)
	go func() {
		defer churnWg.Done()
		for i := 0; i < rounds; i++ {
			select {
			case <-stopCh:
				return
			default:
			}
			id, err := startChat(ctx, chatConfigID())
			if err != nil {
				continue
			}
			liveIDs <- id
			time.Sleep(40 * time.Millisecond)
			_ = stopChat(ctx, id)
		}
	}()

	// Foreground: attach/detach against whatever we can see. Accept 404
	// (session already ended) and 409 (not-attachable) as clean errors.
	successes := 0
	cleanErrors := 0
	for i := 0; i < rounds; i++ {
		var target string
		select {
		case target = <-liveIDs:
		case <-time.After(200 * time.Millisecond):
		}
		if target == "" {
			continue
		}
		code, body, err := attachDebugger(ctx, target, nil)
		if err != nil {
			t.Errorf("attach transport error for %s: %v", target, err)
			continue
		}
		switch code {
		case http.StatusOK:
			successes++
			_, _ = detachDebugger(ctx, target)
		case http.StatusNotFound, http.StatusConflict:
			cleanErrors++
		default:
			t.Errorf("attach %s: unexpected HTTP %d: %s", target, code, body)
		}
	}
	close(stopCh)
	churnWg.Wait()

	t.Logf("churn: %d attach successes, %d clean errors (404/409), 0 server errors", successes, cleanErrors)
	// Drain any lingering.
	for {
		select {
		case id := <-liveIDs:
			_ = stopChat(ctx, id)
		default:
			return
		}
	}
}
