package userinput

import (
	"context"
	"testing"
	"time"
)

func TestTool_Interface(t *testing.T) {
	reqCh := make(chan InputRequest, 1)
	respCh := make(chan string, 1)
	tool := NewTool(reqCh, respCh, "TestAgent", nil)

	if tool.GetName() != "user_input" {
		t.Errorf("Name = %q, want user_input", tool.GetName())
	}
	if tool.GetDescription() == "" {
		t.Error("Description should not be empty")
	}
	schema := tool.GetParametersSchema()
	if schema["type"] != "object" {
		t.Errorf("Schema type = %v, want object", schema["type"])
	}
}

func TestTool_Execute_Success(t *testing.T) {
	reqCh := make(chan InputRequest, 1)
	respCh := make(chan string, 1)
	tool := NewTool(reqCh, respCh, "TestAgent", nil)

	// Simulate TUI responding
	go func() {
		req := <-reqCh
		if req.Question != "What color?" {
			t.Errorf("Question = %q, want 'What color?'", req.Question)
		}
		if req.Default != "blue" {
			t.Errorf("Default = %q, want 'blue'", req.Default)
		}
		respCh <- "red"
	}()

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"question": "What color?",
		"default":  "blue",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result != "red" {
		t.Errorf("Execute result = %q, want 'red'", result)
	}
}

func TestTool_Execute_MissingQuestion(t *testing.T) {
	reqCh := make(chan InputRequest, 1)
	respCh := make(chan string, 1)
	tool := NewTool(reqCh, respCh, "TestAgent", nil)

	_, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Error("Execute with no question should return error")
	}
}

func TestTool_Execute_ContextCancelled(t *testing.T) {
	reqCh := make(chan InputRequest, 1)
	respCh := make(chan string) // unbuffered, will block
	tool := NewTool(reqCh, respCh, "TestAgent", nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	// Drain the request so Execute can proceed to waiting for response
	go func() {
		<-reqCh
	}()

	_, err := tool.Execute(ctx, map[string]interface{}{
		"question": "Will be cancelled",
	})
	if err != context.Canceled {
		t.Errorf("Execute with cancelled ctx returned %v, want context.Canceled", err)
	}
}

func TestTool_Execute_ContextCancelledBeforeSend(t *testing.T) {
	reqCh := make(chan InputRequest) // unbuffered, will block on send
	respCh := make(chan string)
	tool := NewTool(reqCh, respCh, "TestAgent", nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := tool.Execute(ctx, map[string]interface{}{
		"question": "Will never be sent",
	})
	if err != context.Canceled {
		t.Errorf("Execute with pre-cancelled ctx returned %v, want context.Canceled", err)
	}
}
