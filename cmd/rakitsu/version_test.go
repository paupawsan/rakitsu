package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	origVersion, origCommit := Version, BuildCommit
	defer func() { Version, BuildCommit = origVersion, origCommit }()

	cases := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{"dev build, no commit", "dev", "", "dev"},
		{"release build, bare tag + commit", "v0.2.0-alpha.4", "71ccabc", "v0.2.0-alpha.4 (71ccabc)"},
		{"main build, commit already embedded", "v0.2.0-alpha.3.2eff73f", "2eff73f", "v0.2.0-alpha.3.2eff73f"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			Version, BuildCommit = c.version, c.commit
			if got := versionString(); got != c.want {
				t.Errorf("versionString() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestVersionCmd_PrintsLicense checks that `rakitsu version` mentions the
// license, not just the bare version string — a downloaded release binary
// is often the only place a user ever sees this, so it needs to say
// somewhere it isn't a plain-MIT/Apache open-source build.
func TestVersionCmd_PrintsLicense(t *testing.T) {
	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	versionCmd.Run(versionCmd, nil)
	out := buf.String()
	if !strings.Contains(out, "License: BSL 1.1") {
		t.Errorf("rakitsu version should mention the license, got:\n%s", out)
	}
}

// TestRootVersionFlag_PrintsLicense checks the `--version` flag's template
// (root.go's init()) stays in sync with `rakitsu version` above — the two
// are documented as equivalent, so both need the license line.
func TestRootVersionFlag_PrintsLicense(t *testing.T) {
	tmpl := rootCmd.VersionTemplate()
	if !strings.Contains(tmpl, "License: BSL 1.1") {
		t.Errorf("root --version template should mention the license, got:\n%s", tmpl)
	}
}
