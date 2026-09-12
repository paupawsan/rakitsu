package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// HubClient forwards events from a local EventBus to a remote SSE hub
// and polls for debug commands from the hub.
type HubClient struct {
	hubURL    string
	sessionID string
	eventBus  *EventBus
	client    *http.Client
	eventCh   <-chan AgentEvent
	stopCh    chan struct{}
	stopOnce  sync.Once
	wg        sync.WaitGroup

	// Callback for debug commands received from hub
	OnCommand func(action string, data map[string]interface{})
}

// NewHubClient creates a new hub client that forwards events and polls commands.
func NewHubClient(hubURL string, sessionID string, eventBus *EventBus) *HubClient {
	return &HubClient{
		hubURL:    hubURL,
		sessionID: sessionID,
		eventBus:  eventBus,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		stopCh: make(chan struct{}),
	}
}

// Register registers this run session with the hub. mode is "chat" for
// interactive TUI sessions, "run" for one-shot runs (including steerable
// one-shot registrations — cross-session messaging targets either way), and
// "" for legacy callers that don't set mode at all; acceptsMsg mirrors the
// config's settings.session_msg.enabled so the hub can reject deliveries to
// opted-out sessions immediately.
func (c *HubClient) Register(name, query, configPath string, agents []string, mode string, acceptsMsg bool) error {
	body := map[string]interface{}{
		"id":          c.sessionID,
		"name":        name,
		"query":       query,
		"config_path": configPath,
		"agents":      agents,
	}
	if mode != "" {
		body["mode"] = mode
		body["accepts_messages"] = acceptsMsg
	}
	return c.post("/api/hub/register", body)
}

// PostMessageResult reports the outcome of an inbound wait_token'd
// session_message turn back to the hub, so a synchronous sender blocked in
// deliverMessage's remote-wait branch (internal/server/session_message.go)
// can be unblocked via handleHubMessageResult. Best-effort: an unreachable
// hub or an unknown/expired token (the sender already gave up) is a no-op
// from the caller's perspective — the message was already delivered and
// this is purely the reply leg.
func (c *HubClient) PostMessageResult(waitToken, final string, interrupted bool, errText string) error {
	return c.post("/api/hub/message-result", map[string]interface{}{
		"wait_token":  waitToken,
		"final":       final,
		"interrupted": interrupted,
		"error":       errText,
	})
}

// Start begins forwarding events and polling for commands.
func (c *HubClient) Start() {
	c.eventCh = c.eventBus.Subscribe()

	// Event forwarder goroutine — batches events for efficiency
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.forwardEvents()
	}()

	// Command poller goroutine
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.pollCommands()
	}()
}

// Stop deregisters from the hub and stops all goroutines. Idempotent — a
// second call is a no-op instead of panicking on a double close.
func (c *HubClient) Stop(status string) {
	c.stopOnce.Do(func() {
		close(c.stopCh)
		c.eventBus.Unsubscribe(c.eventCh)
		c.wg.Wait()

		// Deregister from hub (best-effort)
		c.post("/api/hub/deregister", map[string]interface{}{
			"id":     c.sessionID,
			"status": status,
		})
	})
}

func (c *HubClient) forwardEvents() {
	batch := make([]AgentEvent, 0, 50)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		data, err := json.Marshal(batch)
		if err != nil {
			return
		}
		resp, err := c.do(http.MethodPost, "/api/hub/ingest", bytes.NewReader(data))
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		batch = batch[:0]
	}

	for {
		select {
		case ev, ok := <-c.eventCh:
			if !ok {
				flush()
				return
			}
			ev.SessionID = c.sessionID
			ev.Payload = RedactEventPayload(ev.EventType, ev.Payload)
			batch = append(batch, ev)
			if len(batch) >= 50 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-c.stopCh:
			flush()
			return
		}
	}
}

func (c *HubClient) pollCommands() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	path := fmt.Sprintf("/api/hub/commands?id=%s", c.sessionID)

	for {
		select {
		case <-ticker.C:
			resp, err := c.do(http.MethodGet, path, nil)
			if err != nil {
				continue
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				fmt.Fprintf(os.Stderr, "hub client: poll commands: hub returned %d\n", resp.StatusCode)
				continue
			}
			var cmds []struct {
				Action string                 `json:"action"`
				Data   map[string]interface{} `json:"data,omitempty"`
			}
			err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&cmds)
			resp.Body.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "hub client: poll commands: decode: %v\n", err)
				continue
			}

			for _, cmd := range cmds {
				if c.OnCommand != nil {
					c.OnCommand(cmd.Action, cmd.Data)
				}
			}
		case <-c.stopCh:
			return
		}
	}
}

// apiTokenEnv mirrors internal/server's constant (auth.go); duplicated so
// this package stays independent of internal/server. The hub gates its
// CLI-report endpoints behind this token, so a CLI
// reporting to a token-protected hub must run with the same token in its
// environment — the cli tool scrubs it from spawned subprocesses.
const apiTokenEnv = "RAKITSU_API_TOKEN"

// do sends one request to the hub, attaching the API token when configured.
func (c *HubClient) do(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.hubURL+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if tok := os.Getenv(apiTokenEnv); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return c.client.Do(req)
}

func (c *HubClient) post(path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.do(http.MethodPost, path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("hub returned %d", resp.StatusCode)
	}
	return nil
}
