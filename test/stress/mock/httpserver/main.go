// Package main provides an OpenAI-compatible HTTP mock LLM server for Docker-sandboxed stress testing.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Tools    []tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream,omitempty"`
}

type message struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"`
	ToolCalls  []toolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type tool struct {
	Type     string   `json:"type"`
	Function function `json:"function"`
}

type function struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   usage    `json:"usage"`
}

type choice struct {
	Index        int     `json:"index"`
	Message      message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

var (
	latencyMs   int
	failureRate float64
	callCount   atomic.Int64
)

func init() {
	if v := os.Getenv("MOCK_LATENCY_MS"); v != "" {
		latencyMs, _ = strconv.Atoi(v)
	}
	if v := os.Getenv("MOCK_FAILURE_RATE"); v != "" {
		failureRate, _ = strconv.ParseFloat(v, 64)
	}
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Simulate latency
	if latencyMs > 0 {
		time.Sleep(time.Duration(latencyMs) * time.Millisecond)
	}

	// Simulate failures
	if failureRate > 0 && rand.Float64() < failureRate {
		http.Error(w, `{"error":{"message":"mock transient error","type":"server_error"}}`, http.StatusInternalServerError)
		return
	}

	if len(req.Messages) == 0 {
		http.Error(w, `{"error":{"message":"messages must not be empty","type":"invalid_request_error"}}`, http.StatusBadRequest)
		return
	}

	n := callCount.Add(1)

	// Determine response: if tools available and last message is not a tool result, call a tool
	hasTools := len(req.Tools) > 0
	lastMsg := req.Messages[len(req.Messages)-1]
	shouldCallTool := hasTools && lastMsg.Role != "tool" && n%3 != 0

	var resp chatResponse
	resp.ID = fmt.Sprintf("chatcmpl-mock-%d", n)
	resp.Object = "chat.completion"
	resp.Created = time.Now().Unix()
	resp.Model = req.Model
	resp.Usage = usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}

	if shouldCallTool {
		toolName := req.Tools[0].Function.Name
		resp.Choices = []choice{{
			Index: 0,
			Message: message{
				Role: "assistant",
				ToolCalls: []toolCall{{
					ID:   fmt.Sprintf("call_%d", n),
					Type: "function",
					Function: functionCall{
						Name:      toolName,
						Arguments: `{"input":"stress-test"}`,
					},
				}},
			},
			FinishReason: "tool_calls",
		}}
	} else {
		resp.Choices = []choice{{
			Index: 0,
			Message: message{
				Role:    "assistant",
				Content: "Mock response: task completed successfully. " + strings.Repeat("x", 200),
			},
			FinishReason: "stop",
		}}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	port := "8999"
	if v := os.Getenv("PORT"); v != "" {
		port = v
	}

	http.HandleFunc("/v1/chat/completions", handleChat)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[{"id":"mock-model","object":"model"}]}`)
	})

	log.Printf("Mock LLM server starting on :%s (latency=%dms, failure_rate=%.2f)", port, latencyMs, failureRate)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
