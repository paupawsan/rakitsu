package chat

import (
	"fmt"
	"strings"
)

// SlashCommand represents a parsed slash command.
type SlashCommand struct {
	Name string
	Args string
}

// ParseSlashCommand parses input starting with '/'.
// Returns nil if the input is not a slash command.
func ParseSlashCommand(input string) *SlashCommand {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return nil
	}
	parts := strings.SplitN(input[1:], " ", 2)
	cmd := &SlashCommand{Name: strings.ToLower(parts[0])}
	if len(parts) > 1 {
		cmd.Args = strings.TrimSpace(parts[1])
	}
	return cmd
}

// HelpText returns the help message for available slash commands.
func HelpText() string {
	return fmt.Sprintf(`Available commands:
  /help              Show this help
  /clear             Clear conversation history
  /usage             Show token/cost/budget usage (Ctrl+U)
  /context           Show what's loaded into each agent's context (Ctrl+X)
  /agent             List agents you can talk to directly (Ctrl+A)
  /agent NAME        Open that agent's thread (Tab cycles threads, Esc returns)
  /push              Push this thread's last reply to the main chat (Ctrl+G)
  /copy              Copy the selected reply (Ctrl+Y; Ctrl+P selects older)
  /retry             Re-run the last query
  /model             List all agents and their current models
  /model AGENT       Show one agent's current model
  /model AGENT MODEL Swap that agent's model for this session
                       (e.g. /model Coder gemini-3.1-flash-lite)
                       Append @PROVIDER to swap provider too
  /exit, /quit       Exit chat`)
}
