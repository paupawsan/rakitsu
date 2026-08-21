package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/paupawsan/rakitsu/internal/config"
)

// ─── A2A JSON-RPC types ───────────────────────────────────────────────────────

type a2aRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  a2aTaskParams `json:"params"`
}

type a2aTaskParams struct {
	ID       string            `json:"id"`
	Message  a2aMessage        `json:"message"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type a2aMessage struct {
	Role  string    `json:"role"`
	Parts []a2aPart `json:"parts"`
}

type a2aPart struct {
	Text string `json:"text"`
}

type a2aResponse struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      int          `json:"id"`
	Result  *a2aTask     `json:"result,omitempty"`
	Error   *a2aRPCError `json:"error,omitempty"`
}

type a2aTask struct {
	ID        string        `json:"id"`
	Status    a2aStatus     `json:"status"`
	Artifacts []a2aArtifact `json:"artifacts,omitempty"`
}

type a2aStatus struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type a2aArtifact struct {
	Parts []a2aPart `json:"parts"`
}

type a2aRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ─── A2ATool ──────────────────────────────────────────────────────────────────

// A2ATool delegates a task to a named agent running in a remote rakitsu
// instance via the Google A2A protocol (tasks/send).
type A2ATool struct {
	name       string
	desc       string
	endpoint   string // base URL of the remote rakitsu, e.g. "http://host:9100"
	agentName  string
	httpClient *http.Client
	nextID     atomic.Int64
}

func (t *A2ATool) GetName() string        { return t.name }
func (t *A2ATool) GetDescription() string { return t.desc }

func (t *A2ATool) GetParametersSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The task or question to delegate to the remote agent",
			},
		},
		"required": []string{"query"},
	}
}

func (t *A2ATool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("a2a tool %q: missing required argument \"query\"", t.name)
	}

	id := int(t.nextID.Add(1))
	taskID := uuid.New().String()

	req := a2aRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tasks/send",
		Params: a2aTaskParams{
			ID:      taskID,
			Message: a2aMessage{Role: "user", Parts: []a2aPart{{Text: query}}},
			Metadata: map[string]string{
				"agent": t.agentName,
			},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("a2a %q: marshal request: %w", t.name, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(t.endpoint, "/")+"/a2a", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("a2a %q: build request: %w", t.name, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("a2a %q: POST: %w", t.name, err)
	}
	defer resp.Body.Close()

	var a2aResp a2aResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&a2aResp); err != nil {
		return "", fmt.Errorf("a2a %q: decode response: %w", t.name, err)
	}

	if a2aResp.Error != nil {
		return "", fmt.Errorf("a2a %q: RPC error %d: %s", t.name, a2aResp.Error.Code, a2aResp.Error.Message)
	}
	if a2aResp.Result == nil {
		return "", fmt.Errorf("a2a %q: empty result", t.name)
	}

	switch a2aResp.Result.Status.State {
	case "completed":
		if len(a2aResp.Result.Artifacts) > 0 && len(a2aResp.Result.Artifacts[0].Parts) > 0 {
			return a2aResp.Result.Artifacts[0].Parts[0].Text, nil
		}
		return "", nil
	case "failed":
		msg := a2aResp.Result.Status.Message
		if msg == "" {
			msg = "remote agent failed"
		}
		return "", fmt.Errorf("a2a %q: agent failed: %s", t.name, msg)
	default:
		return "", fmt.Errorf("a2a %q: unexpected status state %q", t.name, a2aResp.Result.Status.State)
	}
}

// ─── Factory ──────────────────────────────────────────────────────────────────

// NewA2ATool creates an A2ATool from a YAML ToolDefinition.
// Required fields: def.URL (endpoint), def.AgentName (agent name on remote).
func NewA2ATool(def *config.ToolDefinition) (*A2ATool, error) {
	if def.URL == "" {
		return nil, fmt.Errorf("a2a tool %q: endpoint (url) is required", def.Name)
	}
	if def.AgentName == "" {
		return nil, fmt.Errorf("a2a tool %q: agent name is required", def.Name)
	}

	desc := def.Description
	if desc == "" {
		desc = fmt.Sprintf("Delegate to agent %q at %s", def.AgentName, def.URL)
	}

	timeout := time.Duration(def.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &A2ATool{
		name:      def.Name,
		desc:      desc,
		endpoint:  def.URL,
		agentName: def.AgentName,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}
