package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
