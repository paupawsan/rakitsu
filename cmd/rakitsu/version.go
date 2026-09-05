package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the Rakitsu version",
	Long:  `Print the Rakitsu version. Equivalent to 'rakitsu --version'.`,
	Run: func(cmd *cobra.Command, args []string) {
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "rakitsu version %s\n", versionString())
		fmt.Fprintln(out, "License: BSL 1.1 — see LICENSE")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
