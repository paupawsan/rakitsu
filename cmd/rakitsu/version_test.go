package main

import "testing"

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
