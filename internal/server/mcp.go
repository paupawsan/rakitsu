package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/paupawsan/rakitsu/internal/tools"
)

// ─── JSON-RPC types ───────────────────────────────────────────────────────────

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      *int        `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpRPCError `json:"error,omitempty"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ─── MCPServer ────────────────────────────────────────────────────────────────

// MCPServer exposes a ToolRegistry as an MCP HTTP server (Streamable HTTP
// transport, JSON-RPC 2.0). External MCP clients — Claude Desktop, Cursor,
// Zed — POST to /mcp and can discover and invoke all registered tools.
type MCPServer struct {
	registry *tools.ToolRegistry
	version  string
}

// NewMCPServer creates an MCP server backed by the given registry.
// version is the rakitsu version string included in serverInfo.
func NewMCPServer(registry *tools.ToolRegistry, version string) *MCPServer {
	return &MCPServer{registry: registry, version: version}
}

// ServeHTTP handles all MCP JSON-RPC requests. Mount this on any path — the
// recommended convention is /mcp. It has no auth check of its own and
// executes arbitrary registered tools (tools/call) — whichever path this
// gets mounted on MUST be added to requiresAuth (auth.go) in the same
// change, before it's reachable.
func (s *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req mcpRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		s.writeError(w, nil, -32700, "parse error: "+err.Error())
		return
	}

	ctx := r.Context()

	switch req.Method {
	case "initialize":
		s.handleInitialize(w, req)
	case "notifications/initialized":
		// Acknowledgement notification — no response body required.
		w.WriteHeader(http.StatusNoContent)
	case "tools/list":
		s.handleToolsList(w, req)
	case "tools/call":
		s.handleToolsCall(w, ctx, req)
	default:
		s.writeError(w, req.ID, -32601, "method not found: "+req.Method)
	}
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func (s *MCPServer) handleInitialize(w http.ResponseWriter, req mcpRequest) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"serverInfo": map[string]interface{}{
			"name":    "rakitsu",
			"version": s.version,
		},
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
	}
	w.Header().Set("Mcp-Session-Id", uuid.New().String())
	s.writeResult(w, req.ID, result)
}

type mcpToolEntry struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

func (s *MCPServer) handleToolsList(w http.ResponseWriter, req mcpRequest) {
	allTools := s.registry.GetAllTools()
	entries := make([]mcpToolEntry, 0, len(allTools))
	for _, t := range allTools {
		schema := t.GetParametersSchema()
		if schema == nil {
			schema = map[string]interface{}{"type": "object"}
		}
		entries = append(entries, mcpToolEntry{
			Name:        t.GetName(),
			Description: t.GetDescription(),
			InputSchema: schema,
		})
	}
	s.writeResult(w, req.ID, map[string]interface{}{"tools": entries})
}

type mcpToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type mcpContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *MCPServer) handleToolsCall(w http.ResponseWriter, ctx context.Context, req mcpRequest) {
	var params mcpToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(w, req.ID, -32602, "invalid params: "+err.Error())
		return
	}
	if params.Name == "" {
		s.writeError(w, req.ID, -32602, "name is required")
		return
	}

	result, err := s.registry.ExecuteToolCall(ctx, tools.ToolCall{
		Name:      params.Name,
		Arguments: params.Arguments,
	})
	if err != nil {
		s.writeResult(w, req.ID, map[string]interface{}{
			"content": []mcpContentBlock{{Type: "text", Text: err.Error()}},
			"isError": true,
		})
		return
	}
	s.writeResult(w, req.ID, map[string]interface{}{
		"content": []mcpContentBlock{{Type: "text", Text: result}},
		"isError": false,
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (s *MCPServer) writeResult(w http.ResponseWriter, id *int, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mcpResponse{ //nolint:errcheck
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *MCPServer) writeError(w http.ResponseWriter, id *int, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mcpResponse{ //nolint:errcheck
		JSONRPC: "2.0",
		ID:      id,
		Error:   &mcpRPCError{Code: code, Message: msg},
	})
}
