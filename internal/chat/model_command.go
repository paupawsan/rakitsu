package chat

import (
	tea "github.com/charmbracelet/bubbletea"
)

// handleModelCommand is the TUI-side adapter for the `/model` slash command:
// builds a SlashCommandContext from the Model's runner / inner-runner /
// single-agent / BuildLLM / KnownProviders, delegates to the shared
// HandleModelCommand (in slashcmd.go), and appends the reply as a system
// block. The same shared core powers the server-side handler in chat_ws.go
// so both surfaces behave identically.
func (m Model) handleModelCommand(args string) (tea.Model, tea.Cmd) {
	reply := HandleModelCommand(SlashCommandContext{
		Runner:         m.runner,
		InnerRunner:    m.innerRunner,
		SingleAgent:    m.singleAgent,
		BuildLLM:       m.buildLLM,
		KnownProviders: m.knownProviders,
	}, args)

	m.blocks = append(m.blocks, ContentBlock{
		Type: BlockSystem,
		Text: reply,
	})
	m.updateViewport()
	return m, nil
}
