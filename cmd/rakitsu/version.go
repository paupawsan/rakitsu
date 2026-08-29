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
		fmt.Printf("rakitsu version %s\n", versionString())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
