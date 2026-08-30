package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withSessionsSendHub(t *testing.T, url string) {
	t.Helper()
	prev := sessionsSendHub
	sessionsSendHub = url
	t.Cleanup(func() { sessionsSendHub = prev })
}

func withSessionsLiveHub(t *testing.T, url string) {
	t.Helper()
	prev := sessionsLiveHub
	sessionsLiveHub = url
	t.Cleanup(func() { sessionsLiveHub = prev })
}

func TestSessionsSendRejectsSlashInSessionID(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	defer srv.Close()
	withSessionsSendHub(t, srv.URL)

	err := runSessionsSend(nil, []string{"other-session/x", "hello"})
	if err == nil || !strings.Contains(err.Error(), "/") {
		t.Fatalf("err = %v, want a rejection mentioning '/'", err)
	}
	if hit {
		t.Fatal("request reached the server — a slash-containing session-id must be rejected before any request is sent")
	}
}

func TestSessionsSendBearerTokenFallsBackToAPIToken(t *testing.T) {
	t.Setenv("RAKITSU_API_TOKEN", "shared-token")
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]string{"status": "delivered"})
	}))
	defer srv.Close()
	withSessionsSendHub(t, srv.URL)

	if err := runSessionsSend(nil, []string{"target-1", "hello"}); err != nil {
		t.Fatalf("runSessionsSend: %v", err)
	}
	if gotAuth != "Bearer shared-token" {
		t.Errorf("auth header = %q, want fallback to RAKITSU_API_TOKEN", gotAuth)
	}
}

func TestSessionsLiveSendsBearerToken(t *testing.T) {
	t.Setenv("RAKITSU_SESSION_MSG_TOKEN", "sekrit")
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	withSessionsLiveHub(t, srv.URL)

	if err := runSessionsLive(nil, nil); err != nil {
		t.Fatalf("runSessionsLive: %v", err)
	}
	if gotAuth != "Bearer sekrit" {
		t.Errorf("auth header = %q, want %q", gotAuth, "Bearer sekrit")
	}
}
