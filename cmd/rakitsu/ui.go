package main

import (
	"github.com/spf13/cobra"
)

// uiCmd is a deprecated alias for "rakitsu serve".
// All functionality has been unified into rakitsu serve.
var uiCmd = &cobra.Command{
	Use:        "ui",
	Short:      "Alias for 'rakitsu serve' (deprecated — use rakitsu serve)",
	Deprecated: "use 'rakitsu serve' instead — it now includes the full web UI and agent runner",
	Run: func(cmd *cobra.Command, args []string) {
		startServe()
	},
}

func init() {
	rootCmd.AddCommand(uiCmd)
	// Mirror serve flags so existing scripts using rakitsu ui --port still work
	uiCmd.Flags().IntVarP(&servePort, "port", "p", 9100, "Port to run the hub on")
	uiCmd.Flags().StringVar(&serveHost, "host", "localhost", "Host to bind to")
	uiCmd.Flags().StringVar(&serveConfigDir, "config-dir", "", "Extra directory to scan for agent configs")
}
