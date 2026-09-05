package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestInstallHelpBanner_RootOnly checks the banner (containing the version
// string) shows up on the root command's help but not a subcommand's — the
// mark belongs on the CLI's front door, not repeated on every `--help`.
func TestInstallHelpBanner_RootOnly(t *testing.T) {
	root := &cobra.Command{Use: "rakitsu"}
	sub := &cobra.Command{Use: "run", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(sub)
	installHelpBanner(root)

	var rootOut bytes.Buffer
	root.SetOut(&rootOut)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("root --help: %v", err)
	}
	if !strings.Contains(rootOut.String(), "rakitsu "+versionString()) {
		t.Errorf("root --help missing the banner, got:\n%s", rootOut.String())
	}

	var subOut bytes.Buffer
	root.SetOut(&subOut)
	root.SetArgs([]string{"run", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("run --help: %v", err)
	}
	if strings.Contains(subOut.String(), "rakitsu "+versionString()) {
		t.Errorf("subcommand --help should not repeat the banner, got:\n%s", subOut.String())
	}
}
