package a2a

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paupawsan/rakitsu/internal/config"
)

// ─── Mock server helper ───────────────────────────────────────────────────────

// mockA2AServer creates an httptest.Server that handles tasks/send.
// The handler fn receives (agentName, query) and returns (result, errMsg).
// If errMsg != "", the server returns state:"failed". If rpcErr != 0, it
// returns a JSON-RPC error instead.
type mockResponse struct {
	result string
	errMsg string
	rpcErr int // non-zero → return JSON-RPC error with this code
}

func newMockA2AServer(t *testing.T, fn func(agent, query string) mockResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req a2aRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if req.Method != "tasks/send" {
			json.NewEncoder(w).Encode(a2aResponse{ //nolint:errcheck
				JSONRPC: "2.0", ID: req.ID,
				Error: &a2aRPCError{Code: -32601, Message: "method not found"},
			})
			return
		}

		agentName := req.Params.Metadata["agent"]
		var query string
		if len(req.Params.Message.Parts) > 0 {
			query = req.Params.Message.Parts[0].Text
		}

		mr := fn(agentName, query)

		if mr.rpcErr != 0 {
			json.NewEncoder(w).Encode(a2aResponse{ //nolint:errcheck
				JSONRPC: "2.0", ID: req.ID,
				Error: &a2aRPCError{Code: mr.rpcErr, Message: mr.errMsg},
			})
			return
		}

		task := a2aTask{ID: req.Params.ID}
		if mr.errMsg != "" {
			task.Status = a2aStatus{State: "failed", Message: mr.errMsg}
		} else {
			task.Status = a2aStatus{State: "completed"}
			task.Artifacts = []a2aArtifact{{Parts: []a2aPart{{Text: mr.result}}}}
		}
		json.NewEncoder(w).Encode(a2aResponse{JSONRPC: "2.0", ID: req.ID, Result: &task}) //nolint:errcheck
	}))
}

func newTool(t *testing.T, srv *httptest.Server, agentName, desc string) *A2ATool {
	t.Helper()
	tool, err := NewA2ATool(&config.ToolDefinition{
		Name:        "test_tool",
		Description: desc,
		URL:         srv.URL,
		AgentName:   agentName,
	})
	if err != nil {
		t.Fatalf("NewA2ATool: %v", err)
	}
	return tool
}

// ─── TestA2ATool_Execute ──────────────────────────────────────────────────────

func TestA2ATool_Execute(t *testing.T) {
	srv := newMockA2AServer(t, func(agent, query string) mockResponse {
		if agent != "Researcher" {
			return mockResponse{errMsg: "wrong agent: " + agent}
		}
		return mockResponse{result: "research result: " + query}
	})
	defer srv.Close()

	tool := newTool(t, srv, "Researcher", "")
	result, err := tool.Execute(context.Background(), map[string]interface{}{"query": "latest AI trends"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "research result: latest AI trends" {
		t.Errorf("unexpected result: %q", result)
	}
}

// ─── TestA2ATool_AgentFailed ──────────────────────────────────────────────────

func TestA2ATool_AgentFailed(t *testing.T) {
	srv := newMockA2AServer(t, func(_, _ string) mockResponse {
		return mockResponse{errMsg: "agent ran out of context"}
	})
	defer srv.Close()

	tool := newTool(t, srv, "Researcher", "")
	_, err := tool.Execute(context.Background(), map[string]interface{}{"query": "anything"})
	if err == nil {
		t.Fatal("expected error for failed agent, got nil")
	}
	// Must actually come from the "failed" task-status branch, not the
	// JSON-RPC-error branch — a swap or collapse of those two branches
	// would otherwise still leave this passing.
	if !strings.Contains(err.Error(), "agent failed:") {
		t.Errorf("expected an \"agent failed:\" error, got: %v", err)
	}
}

// ─── TestA2ATool_RPCError ─────────────────────────────────────────────────────

func TestA2ATool_RPCError(t *testing.T) {
	srv := newMockA2AServer(t, func(_, _ string) mockResponse {
		return mockResponse{rpcErr: -32001, errMsg: "agent not found"}
	})
	defer srv.Close()

	tool := newTool(t, srv, "Ghost", "")
	_, err := tool.Execute(context.Background(), map[string]interface{}{"query": "hello"})
	if err == nil {
		t.Fatal("expected RPC error, got nil")
	}
	// Must come from the JSON-RPC-error branch, not the "agent failed"
	// task-status branch — see TestA2ATool_AgentFailed's comment.
	if !strings.Contains(err.Error(), "RPC error") {
		t.Errorf("expected an \"RPC error\" error, got: %v", err)
	}
}

// ─── TestA2ATool_TrailingSlashEndpoint ────────────────────────────────────────

func TestA2ATool_TrailingSlashEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(a2aResponse{ //nolint:errcheck
			JSONRPC: "2.0", ID: 1,
			Result: &a2aTask{Status: a2aStatus{State: "completed"}},
		})
	}))
	defer srv.Close()

	// A trailing slash on the configured URL is a plausible config typo
	// (e.g. copied from a browser address bar) — it should still hit
	// "/a2a", not "//a2a".
	tool, err := NewA2ATool(&config.ToolDefinition{
		Name: "test_tool", URL: srv.URL + "/", AgentName: "Researcher",
	})
	if err != nil {
		t.Fatalf("NewA2ATool: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]interface{}{"query": "x"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotPath != "/a2a" {
		t.Errorf("want request path %q, got %q", "/a2a", gotPath)
	}
}

// ─── TestA2ATool_MissingQuery ─────────────────────────────────────────────────

func TestA2ATool_MissingQuery(t *testing.T) {
	// No server needed — error should occur before any HTTP call.
	tool, err := NewA2ATool(&config.ToolDefinition{
		Name:      "test_tool",
		URL:       "http://localhost:9999", // unreachable, should never be called
		AgentName: "Agent",
	})
	if err != nil {
		t.Fatalf("NewA2ATool: %v", err)
	}

	_, err = tool.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing query, got nil")
	}
}

// ─── TestA2ATool_GetParametersSchema ─────────────────────────────────────────

func TestA2ATool_GetParametersSchema(t *testing.T) {
	tool, _ := NewA2ATool(&config.ToolDefinition{Name: "t", URL: "http://x", AgentName: "A"})
	schema := tool.GetParametersSchema()

	if schema["type"] != "object" {
		t.Errorf("want type=object, got %v", schema["type"])
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties not a map")
	}
	query, ok := props["query"].(map[string]interface{})
	if !ok {
		t.Fatal("query property missing")
	}
	if query["type"] != "string" {
		t.Errorf("want query.type=string, got %v", query["type"])
	}
	required, ok := schema["required"].([]string)
	if !ok || len(required) == 0 || required[0] != "query" {
		t.Errorf("expected required=[query], got %v", schema["required"])
	}
}

// ─── TestA2ATool_DefaultDescription ──────────────────────────────────────────

func TestA2ATool_DefaultDescription(t *testing.T) {
	tool, err := NewA2ATool(&config.ToolDefinition{
		Name:      "researcher",
		URL:       "http://host:9100",
		AgentName: "Researcher",
		// Description intentionally empty
	})
	if err != nil {
		t.Fatalf("NewA2ATool: %v", err)
	}
	if tool.GetDescription() == "" {
		t.Error("expected non-empty auto-filled description")
	}
	if tool.GetDescription() == tool.GetName() {
		t.Error("description should not equal name")
	}
}
