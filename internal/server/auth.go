package server

import (
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strings"
)

// apiTokenEnv, when set in the serve process env, makes the control-plane
// endpoints capable of triggering code execution or disclosing local state
// require "Authorization: Bearer <token>". It is also the gate that permits
// binding a non-loopback host: without it, `serve --host 0.0.0.0` refuses to
// start (see requireBindAllowed) because that would expose an unauthenticated,
// RCE-capable API to the network.
//
// One token covers the whole control plane, including cross-session messaging
// (sessionMsgAuthorized accepts either this or RAKITSU_SESSION_MSG_TOKEN).
const apiTokenEnv = "RAKITSU_API_TOKEN"

// withinDir reports whether p is base itself or a genuine descendant of it.
// Anchored on the path separator, not a bare string prefix: a sibling
// directory that merely starts with the same string (e.g. base "/home/alice",
// p "/home/alice-evil") would satisfy strings.HasPrefix(p, base) even though
// it isn't actually under base. Both p and base must already be absolute
// (e.g. via filepath.Abs) — this does no cleaning of its own.
func withinDir(p, base string) bool {
	return p == base || strings.HasPrefix(p, base+string(os.PathSeparator))
}

// isLoopbackHost reports whether host binds only to the local machine.
// The empty host is treated as loopback (net/http defaults are localhost in
// this codebase; the real bind host is always set explicitly by serve/run).
func isLoopbackHost(host string) bool {
	switch host {
	case "", "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// RequireBindAllowed returns a non-nil error when binding the given host would
// expose the control plane to the network without an API token configured.
// Callers should treat a non-nil result as fatal.
func RequireBindAllowed(host string) error {
	if isLoopbackHost(host) {
		return nil
	}
	if os.Getenv(apiTokenEnv) == "" {
		return &bindRefusedError{host: host}
	}
	return nil
}

type bindRefusedError struct{ host string }

func (e *bindRefusedError) Error() string {
	return "refusing to bind non-loopback host " + e.host + " without " + apiTokenEnv +
		" set: this would expose an unauthenticated, code-execution-capable API to the network. " +
		"Set " + apiTokenEnv + "=<token> to enable network exposure, or bind localhost."
}

// requiresAuth reports whether a request targets a control-plane endpoint that
// must be authenticated when an API token is configured. The set is limited to
// endpoints that can execute code, mutate runs/configs, drive the debugger, or
// disclose local filesystem layout — read-only UI data and the static SPA stay
// open so the page can load.
//
// Wiring a new handler onto the mux? It needs adding here too — this is an
// allowlist, not a default-deny, so a forgotten endpoint fails open. MCPServer
// (mcp.go) is the current example: unmounted today so it's not reachable, but
// it executes arbitrary registered tools and has no auth check of its own —
// whichever path it eventually gets mounted on must be added here first.
func requiresAuth(r *http.Request) bool {
	p := r.URL.Path
	switch p {
	case "/api/run", "/api/run/stop",
		"/api/browse", "/api/workdir",
		"/api/configs/upload", "/api/configs/upload-zip", "/api/configs/inline":
		return true
	}
	// Every debugger endpoint is sensitive: it inspects live agent state or
	// injects parameter overrides.
	if strings.HasPrefix(p, "/api/debug/") {
		return true
	}
	// Every chat endpoint is sensitive too: /api/chat lists live session IDs,
	// /api/chat/{id}/... can stop or fork a session or read its transcript,
	// and /ws/chat/{id} drives a session's agent turns over the socket. A
	// client that could enumerate sessions via the unauthenticated list and
	// then drive one via the unauthenticated socket would bypass every other
	// gate here.
	if p == "/api/chat" || strings.HasPrefix(p, "/api/chat/") || strings.HasPrefix(p, "/ws/chat/") {
		return true
	}
	// Same shape, the hub's UI-facing endpoints: /api/hub/sessions (and its
	// /events variant) discloses every active session's query and full
	// buffered event stream, and /api/hub/debug injects debug/param-override
	// commands into one by ID. The CLI-report side of the hub (register,
	// deregister, ingest, commands, message-result) is deliberately NOT
	// gated here — the hub forwarder (internal/telemetry/forwarder.go) sends
	// no Authorization header, so gating those would break CLI<->hub
	// reporting the moment a token is configured.
	if p == "/api/hub/debug" || p == "/api/hub/sessions" || p == "/api/hub/sessions/events" {
		return true
	}
	// Same shape again: /api/sessions/live discloses every live session's
	// ID, name, kind, and status (LiveSessionMeta); /api/runtime/sessions
	// discloses a superset — ID, name, mode, status, plus config ID,
	// started/ended timestamps, agent, and model (session.SessionMeta).
	// Same enumeration risk gated above for /api/hub/sessions, either way.
	if p == "/api/sessions/live" || p == "/api/runtime/sessions" {
		return true
	}
	// Deleting a specific stored config.
	if r.Method == http.MethodDelete && strings.HasPrefix(p, "/api/configs/") {
		return true
	}
	return false
}

// AuthMiddleware enforces the API token on control-plane endpoints when
// apiTokenEnv is set. With no token configured it is a pass-through: the
// loopback trust model applies and requireBindAllowed has already refused any
// non-loopback bind. Wrap this INSIDE CorsMiddleware so CORS preflight
// (OPTIONS) is answered before the token check.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := os.Getenv(apiTokenEnv)
		if token != "" && requiresAuth(r) {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				jsonErrorResponse(w, "unauthorized: missing or invalid API token", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
