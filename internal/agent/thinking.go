package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// ThinkingStore persists full thinking content to disk for debugging.
// Files are written to ~/.rakitsu/thinking/{sessionID}/{agentName}-{iteration}.md.
// All methods are safe to call on a nil receiver (no-op).
type ThinkingStore struct {
	dir string // base directory for this session
}

// NewThinkingStore creates a ThinkingStore rooted at ~/.rakitsu/thinking/{sessionID}/.
// Returns nil (not an error) when the home directory cannot be determined so that
// callers can always treat a nil store as "disabled".
func NewThinkingStore(sessionID string) *ThinkingStore {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dir := filepath.Join(home, ".rakitsu", "thinking", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	return &ThinkingStore{dir: dir}
}

// Save writes thinking content for a single agent iteration to disk.
// A nil receiver is a no-op.
func (s *ThinkingStore) Save(agentName string, iteration int, content string) error {
	if s == nil {
		return nil
	}
	name := fmt.Sprintf("%s-%d.md", sanitizeFileName(agentName), iteration)
	path := filepath.Join(s.dir, name)
	return os.WriteFile(path, []byte(content), 0o644)
}

// sanitizeFileName replaces characters that are invalid in file names.
func sanitizeFileName(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '/' || c == '\\' || c == ':' || c == '*' || c == '?' || c == '"' || c == '<' || c == '>' || c == '|' {
			out[i] = '_'
		} else {
			out[i] = c
		}
	}
	return string(out)
}

const thinkingPlaceholderMax = 300

// ThinkingPlaceholder returns a compact reference to inject into history in place
// of the full thinking content: "[Thought N: <first 300 chars>...]"
func ThinkingPlaceholder(iteration int, content string) string {
	truncated := content
	suffix := ""
	if len(content) > thinkingPlaceholderMax {
		truncated = content[:thinkingPlaceholderMax]
		suffix = "..."
	}
	return fmt.Sprintf("[Thought %d: %s%s]", iteration+1, truncated, suffix)
}
