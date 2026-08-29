// Package shared provides utility functions shared across export sub-packages.
package shared

import "strings"

// SanitizeID converts a display name to a lowercase kebab-case ID.
func SanitizeID(name string) string {
	id := ""
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			id += string(c)
		} else if c >= 'A' && c <= 'Z' {
			lower := string(c - 'A' + 'a')
			if id != "" && id[len(id)-1] != '-' {
				id += "-"
			}
			id += lower
		} else if c == ' ' || c == '-' {
			if id != "" && id[len(id)-1] != '-' {
				id += "-"
			}
		}
	}
	return id
}

// Dedup returns a new slice with duplicate strings removed.
func Dedup(s []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// AppendUnique appends val to slice only if not already present.
func AppendUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}

// ShellQuote wraps a string in single quotes with proper escaping.
// Single quotes within the string are escaped as '\'' (end quote, escaped quote, start quote).
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// SanitizeComment strips embedded newlines from a string so it's safe to
// interpolate into a single-line bash "#" comment — an embedded newline
// would otherwise let the remainder of the string run as an uncommented,
// executable script line.
func SanitizeComment(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ").Replace(s)
}
