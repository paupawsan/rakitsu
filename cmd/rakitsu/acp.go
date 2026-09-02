package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/paupawsan/rakitsu/internal/acp"
	"github.com/paupawsan/rakitsu/internal/config"
	"github.com/spf13/cobra"
)

var acpCmd = &cobra.Command{
	Use:   "acp <config.yaml>",
	Short: "Start ACP (Agent Client Protocol) stdio server",
	Long: `Start an ACP (Agent Client Protocol) server over stdin/stdout.

ACP is the JSON-RPC 2.0 protocol Zed and other editors use to talk to coding
agents. Running this command registers rakitsu as an ACP agent an editor can
discover and invoke — every session it serves runs the config given here,
the same way "rakitsu run <config.yaml> <query>" does for a one-shot run.

Protocol flow:
  editor  -> rakitsu:  initialize
  editor  -> rakitsu:  session/new {"cwd": "...", "mcpServers": []}
  editor  -> rakitsu:  session/prompt {"sessionId": "...", "prompt": [{"type":"text","text":"..."}]}
  rakitsu -> editor:   session/update {...}          (streamed during execution)
  rakitsu -> editor:   (session/prompt response) {"stopReason": "end_turn"}
  editor  -> rakitsu:  session/cancel {"sessionId": "..."}   (notification, no response)

Example (Zed settings.json):
  "agent_servers": {
    "rakitsu": { "command": "/path/to/rakitsu", "args": ["acp", "/path/to/agent.yaml"] }
  }

Examples:
  rakitsu acp agent.yaml               # start ACP server (IDE connects via stdio)
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | rakitsu acp agent.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: runACP,
}

func init() {
	rootCmd.AddCommand(acpCmd)
}

func runACP(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(args[0])
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals gracefully.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	srv := acp.NewServer(cfg, executeConfig)

	fmt.Fprintln(os.Stderr, "rakitsu ACP server ready (stdin/stdout)")

	return srv.Run(ctx, os.Stdin, os.Stdout)
}
