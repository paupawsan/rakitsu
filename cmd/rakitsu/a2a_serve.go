package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/paupawsan/rakitsu/internal/config"
	"github.com/paupawsan/rakitsu/internal/telemetry"
)

// ─── Server-side A2A JSON-RPC types ──────────────────────────────────────────

type srvA2ARequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *int             `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  srvA2ATaskParams `json:"params"`
}

type srvA2ATaskParams struct {
	ID       string            `json:"id"`
	Message  srvA2AMessage     `json:"message"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type srvA2AMessage struct {
	Role  string       `json:"role"`
	Parts []srvA2APart `json:"parts"`
}

type srvA2APart struct {
	Text string `json:"text"`
}

type srvA2AResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      *int          `json:"id,omitempty"`
	Result  *srvA2ATask   `json:"result,omitempty"`
	Error   *srvA2ARPCErr `json:"error,omitempty"`
}

type srvA2ATask struct {
	ID        string           `json:"id"`
	Status    srvA2AStatus     `json:"status"`
	Artifacts []srvA2AArtifact `json:"artifacts,omitempty"`
}

type srvA2AStatus struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type srvA2AArtifact struct {
	Parts []srvA2APart `json:"parts"`
}

type srvA2ARPCErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ─── subConfigForAgent ────────────────────────────────────────────────────────

// subConfigForAgent returns a stripped config containing only the named agent
// and the global tool definitions. Used by the A2A handler to run a single
// remote agent without starting an orchestrator.
func subConfigForAgent(cfg *config.Config, agentName string) (*config.Config, error) {
	for _, a := range cfg.Agents {
		if a.Name == agentName {
			return &config.Config{
				Name:     cfg.Name,
				Settings: cfg.Settings,
				Tools:    cfg.Tools,
				Agents:   []config.AgentDefinition{a},
			}, nil
		}
	}
	return nil, fmt.Errorf("agent %q not found in config", agentName)
}

// ─── a2aHandlerFunc ───────────────────────────────────────────────────────────

// a2aHandlerFunc returns an http.HandlerFunc that implements the Google A2A
// tasks/send method. It runs the named agent from cfg using executeConfig and
// returns the result as an A2A artifact.
//
// Chat mode is explicitly *not* honored here. A2A is an RPC protocol — peers
// call agents expecting a one-shot result. Even if the target config declares
// `interactive: true`, this handler skips runInteractive / ChatHost and goes
// straight to executeConfig. Do NOT change this without auditing every caller
// in the agent graph — an interactive sub-agent mid-pipeline would block the
// whole RPC on a non-existent human prompt.
func a2aHandlerFunc(cfg *config.Config, eventBus *telemetry.EventBus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeA2AJSON := func(resp srvA2AResponse) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck
		}
		writeRPCError := func(id *int, code int, msg string) {
			writeA2AJSON(srvA2AResponse{
				JSONRPC: "2.0", ID: id,
				Error: &srvA2ARPCErr{Code: code, Message: msg},
			})
		}

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req srvA2ARequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			writeRPCError(nil, -32700, "parse error: "+err.Error())
			return
		}

		if req.Method != "tasks/send" {
			writeRPCError(req.ID, -32601, "method not found: "+req.Method)
			return
		}

		agentName := req.Params.Metadata["agent"]
		if agentName == "" {
			writeRPCError(req.ID, -32602, "metadata.agent is required")
			return
		}

		var query string
		if len(req.Params.Message.Parts) > 0 {
			query = req.Params.Message.Parts[0].Text
		}
		if query == "" {
			writeRPCError(req.ID, -32602, "message.parts[0].text (query) is required")
			return
		}

		subCfg, err := subConfigForAgent(cfg, agentName)
		if err != nil {
			writeRPCError(req.ID, -32001, err.Error())
			return
		}

		result, err := executeConfig(r.Context(), subCfg, eventBus, nil, query, nil)

		task := srvA2ATask{ID: req.Params.ID}
		if err != nil {
			task.Status = srvA2AStatus{State: "failed", Message: err.Error()}
		} else {
			task.Status = srvA2AStatus{State: "completed"}
			task.Artifacts = []srvA2AArtifact{{Parts: []srvA2APart{{Text: result}}}}
		}
		writeA2AJSON(srvA2AResponse{JSONRPC: "2.0", ID: req.ID, Result: &task})
	}
}
