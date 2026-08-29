package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// `rakitsu sessions live` + `rakitsu sessions send` — the CLI surface of
// cross-session messaging. Both talk to the hub's live endpoints only, never
// the sessions.json index (which is not concurrency-safe). If the serve
// process requires a bearer token, `send` reads RAKITSU_SESSION_MSG_TOKEN or
// falls back to the server-wide RAKITSU_API_TOKEN — mirrors the server's own
// sessionMsgAuthorized, which accepts either (same pattern as
// internal/tools/sessionmsg's bearerToken; duplicated rather than exporting
// it, since this is a separate CLI-side implementation, not the agent tool).

var (
	sessionsLiveHub string
	sessionsSendHub string
	sessionsSendAs  string
)

var sessionsLiveCmd = &cobra.Command{
	Use:   "live",
	Short: "List live sessions reachable for cross-session messaging",
	Long: `List live chat-mode sessions (web chat + interactive TUI) registered with
the hub. Sessions with ACCEPTS=false have not opted in via
settings.session_msg.enabled and will reject messages.`,
	Args: cobra.NoArgs,
	RunE: runSessionsLive,
}

var sessionsSendCmd = &cobra.Command{
	Use:   "send <session-id> <text>",
	Short: "Send a fire-and-forget message into a live session",
	Long: `Inject a message into a live session (chat, or a steerable one-shot run). The
target session's agent runs a turn on it as if it were a user message (marked
as coming from outside). Fire-and-forget: success means the message was
queued, not that the agent replied.

Examples:
  rakitsu sessions live
  rakitsu sessions send chat-1234-... "status update: build is green"`,
	Args: cobra.ExactArgs(2),
	RunE: runSessionsSend,
}

func init() {
	sessionsLiveCmd.Flags().StringVar(&sessionsLiveHub, "hub", "http://localhost:9100", "hub/server URL")
	sessionsSendCmd.Flags().StringVar(&sessionsSendHub, "hub", "http://localhost:9100", "hub/server URL")
	sessionsSendCmd.Flags().StringVar(&sessionsSendAs, "from-name", "CLI", "sender name shown in the target session")
	sessionsCmd.AddCommand(sessionsLiveCmd, sessionsSendCmd)
}

func runSessionsLive(_ *cobra.Command, _ []string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(sessionsLiveHub, "/")+"/api/sessions/live", nil)
	if err != nil {
		return err
	}
	if tok := sessionsBearerToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("hub not reachable at %s: %w\n  hint: start it with `rakitsu serve`", sessionsLiveHub, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hub returned %d", resp.StatusCode)
	}

	var sessions []struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Location        string `json:"location"`
		Status          string `json:"status"`
		AcceptsMessages bool   `json:"accepts_messages"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&sessions); err != nil {
		return fmt.Errorf("invalid response: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Println("No live sessions.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SESSION ID\tNAME\tLOCATION\tSTATUS\tACCEPTS")
	for _, s := range sessions {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n", s.ID, s.Name, s.Location, s.Status, s.AcceptsMessages)
	}
	return w.Flush()
}

// sessionsBearerToken returns the token for the Authorization header on
// both `sessions live` and `sessions send` requests, preferring the
// message-specific token and falling back to the server-wide one — the
// server accepts either.
func sessionsBearerToken() string {
	if tok := os.Getenv("RAKITSU_SESSION_MSG_TOKEN"); tok != "" {
		return tok
	}
	return os.Getenv("RAKITSU_API_TOKEN")
}

func runSessionsSend(_ *cobra.Command, args []string) error {
	sessionID, text := args[0], args[1]
	if strings.Contains(sessionID, "/") {
		// The server (internal/server/sse.go's handleSessionByID) splits
		// the URL path on "/", taking only the segment before the first
		// one as the target id — a session-id containing "/" would
		// resolve to a DIFFERENT id server-side than intended here.
		// net/http decodes a percent-escaped "/" back to a literal one
		// before that split runs, so url.PathEscape alone would not
		// close this — reject it outright instead.
		return fmt.Errorf("session-id must not contain '/'")
	}

	body, err := json.Marshal(map[string]string{
		"text":            text,
		"from_session_id": "cli",
		"from_name":       sessionsSendAs,
	})
	if err != nil {
		return err
	}
	reqURL := fmt.Sprintf("%s/api/sessions/%s/message", strings.TrimRight(sessionsSendHub, "/"), url.PathEscape(sessionID))
	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if tok := sessionsBearerToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("hub not reachable at %s: %w\n  hint: start it with `rakitsu serve`", sessionsSendHub, err)
	}
	defer resp.Body.Close()

	var out struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&out); err != nil {
		return fmt.Errorf("hub returned %d with unreadable body", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		msg := out.Error
		if msg == "" {
			msg = out.Status
		}
		return fmt.Errorf("delivery failed (%s): %s", out.Status, msg)
	}
	fmt.Printf("delivered to %s\n", sessionID)
	return nil
}
