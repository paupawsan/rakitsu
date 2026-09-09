package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestHubClientPostMessageResult — the wire body matches what
// internal/server/hub.go's handleHubMessageResult expects to decode.
func TestHubClientPostMessageResult(t *testing.T) {
	var gotPath string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHubClient(srv.URL, "cli-1", NewEventBus(8))
	if err := c.PostMessageResult("tok-1", "pong", false, ""); err != nil {
		t.Fatalf("PostMessageResult: %v", err)
	}

	if gotPath != "/api/hub/message-result" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["wait_token"] != "tok-1" || gotBody["final"] != "pong" || gotBody["interrupted"] != false || gotBody["error"] != "" {
		t.Fatalf("body = %+v", gotBody)
	}
}

// TestHubClientStopIsIdempotent: Stop used to close(stopCh) unconditionally,
// so a second call panicked with "close of closed channel".
func TestHubClientStopIsIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHubClient(srv.URL, "cli-1", NewEventBus(8))
	c.Start()

	done := make(chan struct{})
	go func() {
		c.Stop("done")
		c.Stop("done") // must not panic
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return twice within the timeout")
	}
}

// TestHubClientPollCommandsHandlesNonOKAndBadJSON: pollCommands used to
// decode the response body unconditionally, discarding both a non-2xx
// status and the decode error, so a hiccupping hub silently dropped
// commands forever with zero visibility. It must keep polling past a bad
// response and still deliver the next good one.
func TestHubClientPollCommandsHandlesNonOKAndBadJSON(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hub/commands" {
			w.WriteHeader(http.StatusOK)
			return
		}
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			w.WriteHeader(http.StatusInternalServerError)
		case 2:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("not json"))
		default:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]map[string]interface{}{{"action": "ping"}})
		}
	}))
	defer srv.Close()

	c := NewHubClient(srv.URL, "cli-1", NewEventBus(8))
	done := make(chan string, 1)
	c.OnCommand = func(action string, data map[string]interface{}) {
		select {
		case done <- action:
		default:
		}
	}
	c.Start()
	defer c.Stop("test-done")

	select {
	case action := <-done:
		if action != "ping" {
			t.Errorf("action = %q, want ping", action)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("OnCommand never fired — pollCommands likely stopped after the bad response instead of continuing")
	}
}

// TestHubClientSendsAPITokenWhenConfigured: the hub gates its CLI-report
// endpoints (/api/hub/register, ingest, commands, deregister, message-result)
// behind RAKITSU_API_TOKEN, so the forwarder must
// present the token from its own environment on every call — and send no
// Authorization header at all when none is configured.
func TestHubClientSendsAPITokenWhenConfigured(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == "/api/hub/commands" {
			w.Write([]byte("[]"))
		}
	}))
	defer srv.Close()

	t.Setenv("RAKITSU_API_TOKEN", "hub-token")
	bus := NewEventBus(8)
	c := NewHubClient(srv.URL, "cli-1", bus)
	if err := c.Register("n", "q", "cfg.yaml", nil, "run", false); err != nil {
		t.Fatalf("Register: %v", err)
	}
	c.Start()
	bus.Publish(AgentEvent{EventType: EventAgentStart})
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		_, ingested := seen["/api/hub/ingest"]
		_, polled := seen["/api/hub/commands"]
		mu.Unlock()
		if (ingested && polled) || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.Stop("done")
	c.PostMessageResult("tok", "final", false, "")

	mu.Lock()
	for _, p := range []string{"/api/hub/register", "/api/hub/ingest", "/api/hub/commands", "/api/hub/deregister", "/api/hub/message-result"} {
		got, ok := seen[p]
		if !ok {
			t.Errorf("%s was never called", p)
			continue
		}
		if got != "Bearer hub-token" {
			t.Errorf("%s: Authorization = %q, want %q", p, got, "Bearer hub-token")
		}
	}
	delete(seen, "/api/hub/register")
	mu.Unlock()

	t.Setenv("RAKITSU_API_TOKEN", "")
	if err := NewHubClient(srv.URL, "cli-2", NewEventBus(8)).Register("n", "q", "", nil, "", false); err != nil {
		t.Fatalf("Register (no token): %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got := seen["/api/hub/register"]; got != "" {
		t.Errorf("no token configured: Authorization = %q, want empty", got)
	}
}
