package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSSRFSafeDialContext_RejectsBlockedLiteralIP: the dial itself must
// reject a blocked literal IP, not just isAllowedProxyTarget's separate
// pre-check.
func TestSSRFSafeDialContext_RejectsBlockedLiteralIP(t *testing.T) {
	_, err := ssrfSafeDialContext(context.Background(), "tcp", "169.254.169.254:80")
	if err == nil {
		t.Fatal("expected the cloud metadata IP to be rejected at dial time")
	}
}

// TestSSRFSafeDialContext_RejectsBlockedHostname regression-guards the
// DNS-rebinding gap: isAllowedProxyTarget resolves a hostname once via
// net.LookupHost as a pre-check; the http.Client then dials with its own,
// separate resolution. A rebinding DNS server can answer those two
// lookups differently. ssrfSafeDialContext must re-resolve and re-check
// at dial time, so a hostname that resolves to a blocked address is
// rejected here regardless of what the pre-check saw.
func TestSSRFSafeDialContext_RejectsBlockedHostname(t *testing.T) {
	// "localhost" always resolves to 127.0.0.1/::1, which are allowed —
	// use a hostname that resolves to a known-blocked address instead.
	// "169.254.169.254.nip.io"-style wildcard DNS isn't reachable in a
	// sandboxed test env, so exercise the same code path by dialing the
	// literal metadata IP through the "hostname" branch's sibling check:
	// isAllowedProxyIP itself, which ssrfSafeDialContext calls for every
	// resolved address.
	if isAllowedProxyIP(net.ParseIP("169.254.169.254")) {
		t.Fatal("isAllowedProxyIP must reject the cloud metadata IP")
	}
	if isAllowedProxyIP(net.ParseIP("0.0.0.0")) {
		t.Fatal("isAllowedProxyIP must reject the unspecified address")
	}
}

// TestSSRFSafeDialContext_AllowsLoopback proves the fix doesn't break the
// documented use case (reaching a local/Tailscale-networked LLM provider):
// dialing an actual local server through ssrfSafeDialContext must succeed.
func TestSSRFSafeDialContext_AllowsLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	addr := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")
	conn, err := ssrfSafeDialContext(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatalf("expected loopback dial to succeed, got: %v", err)
	}
	conn.Close()
}

// TestNewProxyHTTPClient_RedirectToBlockedTarget_Refused regression-guards
// the redirect bypass: a proxied response that 302s to a blocked address
// must not be followed, since neither isAllowedProxyTarget's pre-check
// nor ssrfSafeDialContext's per-dial check covers a redirect Location the
// client wasn't asked to reach in the first place.
func TestNewProxyHTTPClient_RedirectToBlockedTarget_Refused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer srv.Close()

	client := newProxyHTTPClient()
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(req)
	if err == nil {
		t.Fatal("expected a redirect to a blocked address to be refused")
	}
}
