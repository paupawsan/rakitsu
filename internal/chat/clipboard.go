package chat

import (
	"encoding/base64"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// clearNoticeMsg clears the transient status-bar notice.
type clearNoticeMsg struct{}

// osc52Sequence builds the OSC 52 terminal escape that sets the clipboard to
// s: `ESC ] 52 ; c ; <base64> BEL`. The `c` selection targets the regular
// clipboard. Kept as a pure function so it can be unit-tested without a tty.
func osc52Sequence(s string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(s)) + "\x07"
}

// copyToClipboard returns a Cmd that copies s to the terminal clipboard via an
// OSC 52 escape sequence. OSC 52 sets the *local* clipboard even when rakitsu
// runs over SSH, provided the terminal emulator supports it (iTerm2, kitty,
// WezTerm, tmux with `set-clipboard on`).
//
// The whole sequence is emitted in a single Write so it cannot be split by a
// concurrent bubbletea render frame. OSC 52 produces no visible output and
// does not move the cursor, so writing it mid-render is safe.
func copyToClipboard(s string) tea.Cmd {
	return func() tea.Msg {
		_, _ = os.Stdout.WriteString(osc52Sequence(s))
		return nil
	}
}

// clearNoticeAfter returns a Cmd that emits clearNoticeMsg after d.
func clearNoticeAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearNoticeMsg{} })
}
