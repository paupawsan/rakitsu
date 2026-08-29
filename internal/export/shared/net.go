package shared

import (
	"net/url"
	"strconv"
	"strings"
)

// ExtractHostPort returns the hostname and port from a URL string. Port is 0
// if not present. A bare "host:port" or "host" with no scheme is accepted
// too (existing callers pass config values in either form).
//
// Callers must expand any ${VAR} references before calling this — net/url
// rejects "{"/"}" in the host, so a raw ${VAR} placeholder returns ("", 0)
// rather than a garbage value. (An earlier hand-rolled implementation tried
// to tolerate unexpanded placeholders directly and got userinfo, bare-IPv6,
// and uppercase-scheme URLs wrong as a result — net/url.Parse handles all
// three correctly once the caller has already expanded env vars.)
func ExtractHostPort(rawURL string) (string, int) {
	s := rawURL
	if !strings.Contains(s, "://") {
		// No scheme: url.Parse would otherwise treat the whole string as a
		// relative path, not an authority. The "//" prefix makes it parse
		// "host" or "host:port" as authority even without a scheme.
		s = "//" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", 0
	}
	port := 0
	if p := u.Port(); p != "" {
		port, _ = strconv.Atoi(p)
	}
	return u.Hostname(), port
}

// ExtractHost pulls the hostname from a URL string.
func ExtractHost(rawURL string) string {
	host, _ := ExtractHostPort(rawURL)
	return host
}
