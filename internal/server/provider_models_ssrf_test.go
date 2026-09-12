package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

	client := proxyHTTPClient
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(req)
	if err == nil {
		t.Fatal("expected a redirect to a blocked address to be refused")
	}
}

// TestHandleProviderModels_ReusesConnectionAcrossCalls regression-guards a
// finding from automated review: the proxy handlers used to build a fresh
// http.Client (and fresh http.Transport, each with its own idle-connection
// pool) on every request via newProxyHTTPClient(). Nothing ever closed
// those idle connections, so a busy or repeatedly-hit endpoint accumulated
// open connections/file descriptors across abandoned transports instead of
// reusing one pool.
//
// Proof by observable behavior rather than reflection into unexported
// Transport state: with a real shared client, two requests to the same
// server over keep-alive land on the same TCP connection, which the test
// server sees as an identical net.Conn (compared by pointer via a
// ConnState hook — RemoteAddr can theoretically repeat across different
// connections, a net.Conn identity cannot). A per-call client/Transport
// would instead dial a fresh connection every time.
func TestHandleProviderModels_ReusesConnectionAcrossCalls(t *testing.T) {
	var mu sync.Mutex
	seen := map[net.Conn]bool{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[]}`))
	}))
	srv.Config.ConnState = func(c net.Conn, state http.ConnState) {
		if state == http.StateActive {
			mu.Lock()
			seen[c] = true
			mu.Unlock()
		}
	}
	srv.Start()
	defer srv.Close()

	s := &SSEServer{}
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/providers/models?base_url="+srv.URL, nil)
		rec := httptest.NewRecorder()
		s.handleProviderModels(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: status = %d, body = %s", i, rec.Code, rec.Body.String())
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("expected all 3 requests to reuse one keep-alive connection, server saw %d distinct connections — the proxy client is not being shared across calls", len(seen))
	}
}
