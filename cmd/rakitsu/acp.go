package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/paupawsan/rakitsu/internal/acp"
	"github.com/spf13/cobra"
)

var acpCmd = &cobra.Command{
	Use:   "acp",
	Short: "Start ACP (Agent Client Protocol) stdio server",
	Long: `Start an ACP (Agent Client Protocol) server over stdin/stdout.

ACP is a JSON-RPC 2.0 protocol adopted by JetBrains, Zed, Cursor, and others
as a standard for invoking coding agents directly from IDEs. Running this
command registers rakitsu as an ACP agent that editors can discover and invoke.

Protocol flow:
  IDE → rakitsu:  initialize
  IDE → rakitsu:  agent/run {"config_path": "agent.yaml", "query": "..."}
  rakitsu → IDE:  agent/event {...}   (streamed during execution)
  rakitsu → IDE:  agent/complete {"result": "..."}
  IDE → rakitsu:  agent/cancel {"session_id": "..."}

Examples:
  rakitsu acp                          # start ACP server (IDE connects via stdio)
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | rakitsu acp`,
	RunE: runACP,
}

func init() {
	rootCmd.AddCommand(acpCmd)
}

func runACP(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals gracefully.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	srv := acp.NewServer(executeConfig)

	fmt.Fprintln(os.Stderr, "rakitsu ACP server ready (stdin/stdout)")

	return srv.Run(ctx, os.Stdin, os.Stdout)
}
