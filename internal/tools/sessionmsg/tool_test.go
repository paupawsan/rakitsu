package sessionmsg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewToolsDisabledWithoutHubURL(t *testing.T) {
	if got := NewTools(Deps{SessionID: "s1", AgentName: "A"}); got != nil {
		t.Fatalf("NewTools without HubURL = %v, want nil", got)
	}
}

func TestSendMessagePostsAndReportsDelivery(t *testing.T) {
	t.Setenv("RAKITSU_SESSION_MSG_TOKEN", "sekrit")
	var gotPath, gotAuth string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(map[string]string{"status": "delivered"})
	}))
	defer srv.Close()

	tools := NewTools(Deps{HubURL: srv.URL, SessionID: "chat-me", AgentName: "Kit"})
	if len(tools) != 2 {
		t.Fatalf("NewTools = %d tools, want 2", len(tools))
	}
	send := tools[0]
	out, err := send.Execute(context.Background(), map[string]interface{}{
		"session_id": "chat-target", "text": "hello",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotPath != "/api/sessions/chat-target/message" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer sekrit" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if gotBody["text"] != "hello" || gotBody["from_session_id"] != "chat-me" || gotBody["from_name"] != "Kit" {
		t.Fatalf("body = %+v", gotBody)
	}
	if !strings.Contains(out, "delivered") {
		t.Fatalf("output = %q", out)
	}
}

func TestSendMessageSurfacesServerRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"status": "rejected", "error": "session does not accept messages"})
	}))
	defer srv.Close()

	send := NewTools(Deps{HubURL: srv.URL, SessionID: "chat-me", AgentName: "Kit"})[0]
	_, err := send.Execute(context.Background(), map[string]interface{}{
		"session_id": "chat-target", "text": "hello",
	})
	if err == nil || !strings.Contains(err.Error(), "rejected") || !strings.Contains(err.Error(), "does not accept") {
		t.Fatalf("Execute err = %v, want rejection passthrough", err)
	}
}

func TestSendMessageWaitReturnsReply(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "delivered", "waited": true, "reply": "pong", "interrupted": false,
		})
	}))
	defer srv.Close()

	send := NewTools(Deps{HubURL: srv.URL, SessionID: "chat-me", AgentName: "Kit"})[0]
	out, err := send.Execute(context.Background(), map[string]interface{}{
		"session_id": "chat-target", "text": "ping", "wait": true, "timeout_seconds": float64(10),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "pong" {
		t.Fatalf("output = %q, want the reply text verbatim", out)
	}
	if gotBody["wait"] != true {
		t.Fatalf("outbound body = %+v, want wait=true", gotBody)
	}
	if gotBody["timeout_ms"] != float64(10000) {
		t.Fatalf("outbound body = %+v, want timeout_ms=10000", gotBody)
	}
}

func TestSendMessageWaitTurnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "delivered", "waited": true, "interrupted": true, "turn_error": "boom",
		})
	}))
	defer srv.Close()

	send := NewTools(Deps{HubURL: srv.URL, SessionID: "chat-me", AgentName: "Kit"})[0]
	_, err := send.Execute(context.Background(), map[string]interface{}{
		"session_id": "chat-target", "text": "ping", "wait": true,
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Execute err = %v, want it to surface the turn error", err)
	}
}

func TestSendMessageWaitTimeoutFallsBackToCannedMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "delivered", "waited": false, "note": "reply not received within timeout; poll GET /api/chat/{id}/tree",
		})
	}))
	defer srv.Close()

	send := NewTools(Deps{HubURL: srv.URL, SessionID: "chat-me", AgentName: "Kit"})[0]
	out, err := send.Execute(context.Background(), map[string]interface{}{
		"session_id": "chat-target", "text": "ping", "wait": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "delivered") || !strings.Contains(out, "poll GET") {
		t.Fatalf("output = %q, want a delivered/fallback message carrying the server's note", out)
	}
}

func TestSendMessageWaitUsesLongerClientTimeout(t *testing.T) {
	tools := NewTools(Deps{HubURL: "http://localhost:1", SessionID: "s1", AgentName: "A"})
	send := tools[0].(*SendMessageTool)
	if send.d.WaitHTTPClient.Timeout <= send.d.HTTPClient.Timeout {
		t.Fatalf("WaitHTTPClient.Timeout = %v, want it longer than HTTPClient.Timeout = %v", send.d.WaitHTTPClient.Timeout, send.d.HTTPClient.Timeout)
	}
}

func TestSendMessageRefusesSelfSend(t *testing.T) {
	send := NewTools(Deps{HubURL: "http://localhost:1", SessionID: "chat-me", AgentName: "Kit"})[0]
	_, err := send.Execute(context.Background(), map[string]interface{}{
		"session_id": "chat-me", "text": "hello",
	})
	if err == nil || !strings.Contains(err.Error(), "this session") {
		t.Fatalf("self-send err = %v, want refusal", err)
	}
}

func TestSendMessageRejectsSlashInSessionID(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		json.NewEncoder(w).Encode(map[string]string{"status": "delivered"})
	}))
	defer srv.Close()
	send := NewTools(Deps{HubURL: srv.URL, SessionID: "chat-me", AgentName: "Kit"})[0]
	_, err := send.Execute(context.Background(), map[string]interface{}{
		"session_id": "other-session/x", "text": "hello",
	})
	if err == nil || !strings.Contains(err.Error(), "/") {
		t.Fatalf("err = %v, want a rejection mentioning '/'", err)
	}
	if hit {
		t.Fatal("request reached the server — a slash-containing session_id must be rejected before any request is sent")
	}
}

func TestSendMessageSelfSendBypassViaSlashIsRejected(t *testing.T) {
	// A crafted "<own-id>/x" is not literally equal to SessionID, so the
	// plain self-send comparison alone would miss it — but the server
	// splits the path on "/" and would resolve the target back to the
	// caller's own session. The slash guard must reject this before the
	// self-send comparison ever runs.
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	defer srv.Close()
	send := NewTools(Deps{HubURL: srv.URL, SessionID: "chat-me", AgentName: "Kit"})[0]
	_, err := send.Execute(context.Background(), map[string]interface{}{
		"session_id": "chat-me/x", "text": "hello",
	})
	if err == nil {
		t.Fatal("expected an error for a slash-containing session_id that would resolve back to the caller's own session")
	}
	if hit {
		t.Fatal("request reached the server — self-send-via-slash must be rejected client-side")
	}
}

func TestListSessionsMarksOwnRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sessions/live" {
			t.Errorf("path = %q", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{"id": "chat-me", "name": "self", "kind": "chat", "location": "local", "status": "running", "accepts_messages": true},
			{"id": "tui-1", "name": "other", "kind": "chat", "location": "remote", "status": "running", "accepts_messages": false},
		})
	}))
	defer srv.Close()

	list := NewTools(Deps{HubURL: srv.URL, SessionID: "chat-me", AgentName: "Kit"})[1]
	out, err := list.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "(you") {
		t.Fatalf("own row not marked: %q", out)
	}
	if !strings.Contains(out, "tui-1 | other | chat | remote | false") {
		t.Fatalf("other row missing/mangled: %q", out)
	}
}

func TestListSessionsShowsKind(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":"run-1","name":"pipe","location":"remote","kind":"one-shot","accepts_messages":true}]`))
	}))
	defer srv.Close()
	tl := NewTools(Deps{HubURL: srv.URL, SessionID: "me", AgentName: "a"})
	out, err := tl[1].Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "one-shot") {
		t.Fatalf("kind column missing: %q", out)
	}
}

func TestSendMessageMailboxedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"mailboxed"}`))
	}))
	defer srv.Close()
	tl := NewTools(Deps{HubURL: srv.URL, SessionID: "me", AgentName: "a"})
	out, err := tl[0].Execute(context.Background(), map[string]interface{}{"session_id": "cc-1", "text": "done"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "inbox") {
		t.Fatalf("mailboxed send must mention the hub inbox: %q", out)
	}
}

func TestListSessionsSendsBearerToken(t *testing.T) {
	t.Setenv("RAKITSU_SESSION_MSG_TOKEN", "sekrit")
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	tl := NewTools(Deps{HubURL: srv.URL, SessionID: "me", AgentName: "a"})
	if _, err := tl[1].Execute(context.Background(), nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotAuth != "Bearer sekrit" {
		t.Fatalf("list_sessions auth header = %q, want %q", gotAuth, "Bearer sekrit")
	}
}

func TestBearerTokenFallsBackToAPIToken(t *testing.T) {
	// No RAKITSU_SESSION_MSG_TOKEN — only the server-wide RAKITSU_API_TOKEN.
	// Both tools must still authenticate, since the server's own
	// sessionMsgAuthorized accepts either.
	t.Setenv("RAKITSU_API_TOKEN", "shared-token")
	var sendAuth, listAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			sendAuth = r.Header.Get("Authorization")
			json.NewEncoder(w).Encode(map[string]string{"status": "delivered"})
			return
		}
		listAuth = r.Header.Get("Authorization")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	tl := NewTools(Deps{HubURL: srv.URL, SessionID: "me", AgentName: "a"})
	if _, err := tl[0].Execute(context.Background(), map[string]interface{}{"session_id": "cc-1", "text": "hi"}); err != nil {
		t.Fatalf("send_message Execute: %v", err)
	}
	if _, err := tl[1].Execute(context.Background(), nil); err != nil {
		t.Fatalf("list_sessions Execute: %v", err)
	}
	if sendAuth != "Bearer shared-token" {
		t.Errorf("send_message auth header = %q, want fallback to RAKITSU_API_TOKEN", sendAuth)
	}
	if listAuth != "Bearer shared-token" {
		t.Errorf("list_sessions auth header = %q, want fallback to RAKITSU_API_TOKEN", listAuth)
	}
}

func TestBearerTokenPrefersSessionMsgToken(t *testing.T) {
	t.Setenv("RAKITSU_SESSION_MSG_TOKEN", "specific")
	t.Setenv("RAKITSU_API_TOKEN", "general")
	if got := bearerToken(); got != "specific" {
		t.Fatalf("bearerToken() = %q, want the message-specific token to take precedence", got)
	}
}
