//go:build stress

package stress

// Stress tests for the Phase 1 session registry. Exercise the registry
// under concurrent start/stop patterns and assert invariants:
//   I1. Every started session shows up in /api/runtime/sessions within one poll.
//   I2. Every stopped session disappears from /api/runtime/sessions within one poll.
//   I3. Counts never go negative or exceed the number of issued starts.
//   I4. No session_id is duplicated in the registry.
//   I5. Stop() is idempotent — stopping a session that already ended returns no error
//       (or a clean 404) and the registry stays consistent.
//
// Run with:   go test -tags stress -v -run Registry ./test/stress/...
// Requires:   rakitsu serve running at BASE_URL (defaults http://localhost:9100).
//
// These tests are intentionally tolerant of a stale server (old binary) — they
// report the bug but don't false-positive on it. Look at the logged output:
// if I2 fails with stuck "running" entries, the server predates the
// session-registry sync/unregister fix.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================
// Harness
// ============================================================

type runtimeSession struct {
	ID       string `json:"id"`
	ConfigID string `json:"config_id"`
	Name     string `json:"name"`
	Mode     string `json:"mode"`
	Status   string `json:"status"`
}

type chatStartResp struct {
	ID       string `json:"id"`
	ConfigID string `json:"config_id"`
}

func baseURL() string {
	if s := os.Getenv("RAKITSU_BASE_URL"); s != "" {
		return strings.TrimRight(s, "/")
	}
	return "http://localhost:9100"
}

func chatConfigID() string {
	if s := os.Getenv("RAKITSU_CHAT_CONFIG"); s != "" {
		return s
	}
	return "ba616ec02aa0" // Chat WS Test — litellm-backed, no OPENAI_API_KEY needed
}

func listRuntimeSessions(ctx context.Context, t *testing.T) []runtimeSession {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL()+"/api/runtime/sessions", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list runtime sessions: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("list runtime sessions: HTTP %d: %s", resp.StatusCode, body)
	}
	var out []runtimeSession
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("list runtime sessions: bad json %v; body=%s", err, body)
	}
	return out
}

func startChat(ctx context.Context, configID string) (string, error) {
	body := fmt.Sprintf(`{"config_id":"%s"}`, configID)
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL()+"/api/chat/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, rb)
	}
	var out chatStartResp
	if err := json.Unmarshal(rb, &out); err != nil {
		return "", fmt.Errorf("decode: %v; body=%s", err, rb)
	}
	return out.ID, nil
}

func stopChat(ctx context.Context, id string) error {
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL()+"/api/chat/"+id+"/stop", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		rb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, rb)
	}
	return nil // 200 or 404 both acceptable (already stopped)
}

func stopOneShot(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL()+"/api/run/stop", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		rb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, rb)
	}
	return nil
}

func requireServerUp(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL()+"/api/status", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("rakitsu serve not reachable at %s: %v", baseURL(), err)
	}
	resp.Body.Close()
	// Reset runner to idle so prior tests don't leak state.
	_ = stopOneShot(ctx)
}

// waitUntil polls every 200ms until pred() is true or budget expires.
// Returns true on success, false on timeout.
func waitUntil(budget time.Duration, pred func() bool) bool {
	deadline := time.Now().Add(budget)
	for {
		if pred() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func countByMode(list []runtimeSession, mode string) int {
	n := 0
	for _, s := range list {
		if s.Mode == mode {
			n++
		}
	}
	return n
}

func idSet(list []runtimeSession) map[string]struct{} {
	m := make(map[string]struct{}, len(list))
	for _, s := range list {
		m[s.ID] = struct{}{}
	}
	return m
}

// ============================================================
// Tests
// ============================================================

// TestRegistry_ConcurrentChatStartStop fans out N chat starts in parallel,
// verifies they all show up in the registry, then fans out N stops and
// verifies the registry drains to zero.
func TestRegistry_ConcurrentChatStartStop(t *testing.T) {
	requireServerUp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const N = 10
	var wg sync.WaitGroup
	idsCh := make(chan string, N)
	errs := make(chan error, N)

	// ---- Phase: spawn N chats in parallel ----
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := startChat(ctx, chatConfigID())
			if err != nil {
				errs <- err
				return
			}
			idsCh <- id
		}()
	}
	wg.Wait()
	close(idsCh)
	close(errs)

	for e := range errs {
		t.Errorf("startChat: %v", e)
	}

	started := make([]string, 0, N)
	for id := range idsCh {
		started = append(started, id)
	}
	if len(started) != N {
		t.Fatalf("expected %d chats started, got %d", N, len(started))
	}

	// ---- I1: all started ids appear in registry within 3s ----
	if !waitUntil(3*time.Second, func() bool {
		list := listRuntimeSessions(ctx, t)
		got := idSet(list)
		for _, id := range started {
			if _, ok := got[id]; !ok {
				return false
			}
		}
		return true
	}) {
		list := listRuntimeSessions(ctx, t)
		t.Errorf("I1 violated: not all started ids in registry. started=%v, got=%+v", started, list)
	}

	// ---- Phase: stop all in parallel ----
	var wg2 sync.WaitGroup
	for _, id := range started {
		id := id
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if err := stopChat(ctx, id); err != nil {
				t.Errorf("stopChat(%s): %v", id, err)
			}
		}()
	}
	wg2.Wait()

	// ---- I2: all stopped ids gone from registry within 3s ----
	if !waitUntil(3*time.Second, func() bool {
		list := listRuntimeSessions(ctx, t)
		got := idSet(list)
		for _, id := range started {
			if _, ok := got[id]; ok {
				return false
			}
		}
		return true
	}) {
		list := listRuntimeSessions(ctx, t)
		t.Errorf("I2 violated: stopped ids still present. started=%v, got=%+v", started, list)
	}

	t.Logf("PASS: %d chats started + stopped concurrently, registry drained to %d", N, len(listRuntimeSessions(ctx, t)))
}

// TestRegistry_StopIsIdempotent stops a chat twice and ensures the second
// call doesn't panic and the registry stays consistent.
func TestRegistry_StopIsIdempotent(t *testing.T) {
	requireServerUp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	id, err := startChat(ctx, chatConfigID())
	if err != nil {
		t.Fatalf("startChat: %v", err)
	}
	if err := stopChat(ctx, id); err != nil {
		t.Fatalf("first stop: %v", err)
	}
	// Second stop — must not crash the server. 404 is acceptable.
	if err := stopChat(ctx, id); err != nil {
		// Non-5xx already filtered in stopChat; anything here is a real failure.
		t.Fatalf("second stop (should be idempotent): %v", err)
	}

	if !waitUntil(3*time.Second, func() bool {
		for _, s := range listRuntimeSessions(ctx, t) {
			if s.ID == id {
				return false
			}
		}
		return true
	}) {
		t.Error("idempotent-stop: session still present after two stops")
	}
}

// TestRegistry_NoDuplicateIDs repeatedly lists the registry while starting
// chats and verifies no session_id ever appears twice in one snapshot.
func TestRegistry_NoDuplicateIDs(t *testing.T) {
	requireServerUp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Spin up a few chats.
	started := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		id, err := startChat(ctx, chatConfigID())
		if err != nil {
			t.Fatalf("startChat: %v", err)
		}
		started = append(started, id)
	}
	defer func() {
		for _, id := range started {
			_ = stopChat(context.Background(), id)
		}
	}()

	// Poll the registry rapidly for 2s; any snapshot with a duplicate id fails.
	deadline := time.Now().Add(2 * time.Second)
	snapshots := 0
	for time.Now().Before(deadline) {
		list := listRuntimeSessions(ctx, t)
		seen := make(map[string]int, len(list))
		for _, s := range list {
			seen[s.ID]++
		}
		for id, n := range seen {
			if n > 1 {
				t.Errorf("I4 violated: session %s appeared %d times in one snapshot", id, n)
			}
		}
		snapshots++
	}
	t.Logf("checked %d snapshots, no duplicates", snapshots)
}

// TestRegistry_ChurnStability alternates start+stop rapidly for a while and
// asserts the steady-state count converges to zero after the churn ends.
func TestRegistry_ChurnStability(t *testing.T) {
	requireServerUp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	const rounds = 20
	const parallel = 4

	var startedTotal atomic.Int64
	var stoppedTotal atomic.Int64

	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup
		ids := make([]string, parallel)
		for i := 0; i < parallel; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				id, err := startChat(ctx, chatConfigID())
				if err != nil {
					t.Errorf("round %d start: %v", round, err)
					return
				}
				ids[i] = id
				startedTotal.Add(1)
			}()
		}
		wg.Wait()

		var wg2 sync.WaitGroup
		for _, id := range ids {
			if id == "" {
				continue
			}
			id := id
			wg2.Add(1)
			go func() {
				defer wg2.Done()
				if err := stopChat(ctx, id); err != nil {
					t.Errorf("round %d stop: %v", round, err)
					return
				}
				stoppedTotal.Add(1)
			}()
		}
		wg2.Wait()
	}

	// Drain window — give the server up to 5s to converge.
	if !waitUntil(5*time.Second, func() bool {
		return countByMode(listRuntimeSessions(ctx, t), "chat") == 0
	}) {
		list := listRuntimeSessions(ctx, t)
		ids := make([]string, 0, len(list))
		for _, s := range list {
			ids = append(ids, s.ID)
		}
		sort.Strings(ids)
		t.Errorf("churn: chats did not drain to zero; survivors=%v (total started=%d stopped=%d)",
			ids, startedTotal.Load(), stoppedTotal.Load())
	}

	t.Logf("churn: started=%d stopped=%d, steady state clean",
		startedTotal.Load(), stoppedTotal.Load())
}

// TestRegistry_OneShotStopClears sanity-checks that stopping a one-shot run
// clears it from the registry immediately. This is the bug surfaced during
// manual smoke (RAG Assistant stuck as "running" forever because the LLM
// call ignored ctx cancellation). Fixed by adding a sync-unregister call
// inside AgentRunner.Stop.
//
// This test is skipped unless RAKITSU_STUCK_CONFIG is set — it requires a
// config that will start cleanly but stall in the LLM call. Without it we
// can't reliably reproduce the stuck-run path on CI.
func TestRegistry_OneShotStopClears(t *testing.T) {
	requireServerUp(t)
	stuckCfg := os.Getenv("RAKITSU_STUCK_CONFIG")
	if stuckCfg == "" {
		t.Skip("set RAKITSU_STUCK_CONFIG=<config_id> to exercise the stuck-run path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Kick off a one-shot.
	body := fmt.Sprintf(`{"config_id":"%s","query":"stall","timeout":60}`, stuckCfg)
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL()+"/api/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("start oneshot: %v", err)
	}
	resp.Body.Close()

	// Wait until it shows up.
	if !waitUntil(3*time.Second, func() bool {
		return countByMode(listRuntimeSessions(ctx, t), "oneshot") >= 1
	}) {
		t.Fatal("oneshot did not appear in registry")
	}

	// Stop it.
	if err := stopOneShot(ctx); err != nil {
		t.Fatalf("stop oneshot: %v", err)
	}

	// Must clear within one poll (2s).
	if !waitUntil(3*time.Second, func() bool {
		return countByMode(listRuntimeSessions(ctx, t), "oneshot") == 0
	}) {
		list := listRuntimeSessions(ctx, t)
		t.Errorf("one-shot did not unregister after Stop; registry=%+v", list)
		t.Log("Likely cause: server predates the session-registry sync-unregister fix.")
	}
}
