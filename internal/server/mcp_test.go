package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	neutools "github.com/paupawsan/rakitsu/internal/tools"
)

// ─── Stub tool ────────────────────────────────────────────────────────────────

type stubTool struct {
	name   string
	desc   string
	result string
	err    error
}

func (t *stubTool) GetName() string        { return t.name }
func (t *stubTool) GetDescription() string { return t.desc }
func (t *stubTool) GetParametersSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"input": map[string]interface{}{"type": "string"}},
	}
}
func (t *stubTool) Execute(_ context.Context, _ map[string]interface{}) (string, error) {
	return t.result, t.err
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

func newTestMCPRegistry(stubs ...*stubTool) *neutools.ToolRegistry {
	reg := neutools.NewToolRegistry()
	for _, s := range stubs {
		reg.RegisterTool(s)
	}
	return reg
}

func mcpPost(t *testing.T, srv *httptest.Server, body interface{}) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(srv.URL+"/mcp", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	return resp
}

func decodeResp(t *testing.T, resp *http.Response) mcpResponse {
	t.Helper()
	defer resp.Body.Close()
	var r mcpResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return r
}

// ─── TestMCP_Initialize ───────────────────────────────────────────────────────

func TestMCP_Initialize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(NewMCPServer(newTestMCPRegistry(), "test-version").ServeHTTP))
	defer srv.Close()

	id := 1
	resp := mcpPost(t, srv, mcpRequest{
		JSONRPC: "2.0", ID: &id, Method: "initialize",
		Params: json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"clientInfo":{"name":"test","version":"1.0"}}`),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if sid := resp.Header.Get("Mcp-Session-Id"); sid == "" {
		t.Error("expected Mcp-Session-Id header to be set")
	}

	r := decodeResp(t, resp)
	if r.Error != nil {
		t.Fatalf("unexpected error: %+v", r.Error)
	}

	b, _ := json.Marshal(r.Result)
	var result map[string]interface{}
	json.Unmarshal(b, &result) //nolint:errcheck
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("wrong protocolVersion: %v", result["protocolVersion"])
	}
	si, _ := result["serverInfo"].(map[string]interface{})
	if si["name"] != "rakitsu" {
		t.Errorf("wrong server name: %v", si["name"])
	}
	if si["version"] != "test-version" {
		t.Errorf("wrong server version: %v", si["version"])
	}
}

// ─── TestMCP_ToolsList ────────────────────────────────────────────────────────

func TestMCP_ToolsList(t *testing.T) {
	reg := newTestMCPRegistry(
		&stubTool{name: "greet", desc: "Say hello"},
		&stubTool{name: "farewell", desc: "Say goodbye"},
	)
	srv := httptest.NewServer(http.HandlerFunc(NewMCPServer(reg, "v1").ServeHTTP))
	defer srv.Close()

	id := 2
	r := decodeResp(t, mcpPost(t, srv, mcpRequest{JSONRPC: "2.0", ID: &id, Method: "tools/list"}))
	if r.Error != nil {
		t.Fatalf("unexpected error: %+v", r.Error)
	}

	b, _ := json.Marshal(r.Result)
	var result struct {
		Tools []mcpToolEntry `json:"tools"`
	}
	json.Unmarshal(b, &result) //nolint:errcheck

	if len(result.Tools) != 2 {
		t.Fatalf("want 2 tools, got %d", len(result.Tools))
	}
	names := map[string]bool{}
	for _, te := range result.Tools {
		names[te.Name] = true
		if te.InputSchema == nil {
			t.Errorf("tool %q has nil inputSchema", te.Name)
		}
	}
	if !names["greet"] || !names["farewell"] {
		t.Errorf("missing expected tools in: %v", names)
	}
}

// ─── TestMCP_ToolsCall ────────────────────────────────────────────────────────

func TestMCP_ToolsCall(t *testing.T) {
	reg := newTestMCPRegistry(&stubTool{name: "echo", desc: "Echo input", result: "echoed!"})
	srv := httptest.NewServer(http.HandlerFunc(NewMCPServer(reg, "v1").ServeHTTP))
	defer srv.Close()

	id := 3
	r := decodeResp(t, mcpPost(t, srv, mcpRequest{
		JSONRPC: "2.0", ID: &id, Method: "tools/call",
		Params: json.RawMessage(`{"name":"echo","arguments":{"input":"hello"}}`),
	}))
	if r.Error != nil {
		t.Fatalf("unexpected error: %+v", r.Error)
	}

	b, _ := json.Marshal(r.Result)
	var result struct {
		Content []mcpContentBlock `json:"content"`
		IsError bool              `json:"isError"`
	}
	json.Unmarshal(b, &result) //nolint:errcheck

	if result.IsError {
		t.Error("isError should be false")
	}
	if len(result.Content) != 1 || result.Content[0].Text != "echoed!" {
		t.Errorf("unexpected content: %+v", result.Content)
	}
}

// ─── TestMCP_ToolsCall_NotFound ───────────────────────────────────────────────

func TestMCP_ToolsCall_NotFound(t *testing.T) {
	reg := newTestMCPRegistry() // empty
	srv := httptest.NewServer(http.HandlerFunc(NewMCPServer(reg, "v1").ServeHTTP))
	defer srv.Close()

	id := 4
	r := decodeResp(t, mcpPost(t, srv, mcpRequest{
		JSONRPC: "2.0", ID: &id, Method: "tools/call",
		Params: json.RawMessage(`{"name":"no_such_tool","arguments":{}}`),
	}))
	if r.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", r.Error)
	}
	// Tool-not-found surfaces as isError:true in content (not a JSON-RPC protocol error).
	b, _ := json.Marshal(r.Result)
	var result struct {
		IsError bool `json:"isError"`
	}
	json.Unmarshal(b, &result) //nolint:errcheck
	if !result.IsError {
		t.Error("expected isError:true for unknown tool")
	}
}

// ─── TestMCP_NotificationsInitialized ────────────────────────────────────────

func TestMCP_NotificationsInitialized(t *testing.T) {
	reg := newTestMCPRegistry()
	srv := httptest.NewServer(http.HandlerFunc(NewMCPServer(reg, "v1").ServeHTTP))
	defer srv.Close()

	resp := mcpPost(t, srv, mcpRequest{JSONRPC: "2.0", Method: "notifications/initialized"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("want 204, got %d", resp.StatusCode)
	}
}

// ─── TestMCP_UnknownMethod ────────────────────────────────────────────────────

func TestMCP_UnknownMethod(t *testing.T) {
	reg := newTestMCPRegistry()
	srv := httptest.NewServer(http.HandlerFunc(NewMCPServer(reg, "v1").ServeHTTP))
	defer srv.Close()

	id := 5
	r := decodeResp(t, mcpPost(t, srv, mcpRequest{JSONRPC: "2.0", ID: &id, Method: "no_such_method"}))
	if r.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if r.Error.Code != -32601 {
		t.Errorf("want code -32601, got %d", r.Error.Code)
	}
}

// ─── TestMCP_NonPOST ──────────────────────────────────────────────────────────

func TestMCP_NonPOST(t *testing.T) {
	reg := newTestMCPRegistry()
	srv := httptest.NewServer(http.HandlerFunc(NewMCPServer(reg, "v1").ServeHTTP))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/mcp")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", resp.StatusCode)
	}
}
