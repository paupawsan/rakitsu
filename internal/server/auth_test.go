package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLoopbackHost(t *testing.T) {
	cases := map[string]bool{
		"":             true,
		"localhost":    true,
		"127.0.0.1":    true,
		"::1":          true,
		"[::1]":        true,
		"127.0.0.5":    true,
		"0.0.0.0":      false,
		"192.168.1.99": false,
		"10.0.0.1":     false,
		"example.com":  false,
	}
	for host, want := range cases {
		if got := isLoopbackHost(host); got != want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestRequireBindAllowed(t *testing.T) {
	// Loopback never needs a token.
	t.Setenv(apiTokenEnv, "")
	if err := RequireBindAllowed("localhost"); err != nil {
		t.Errorf("loopback bind should be allowed without token, got %v", err)
	}
	// Non-loopback without a token is refused.
	if err := RequireBindAllowed("0.0.0.0"); err == nil {
		t.Error("non-loopback bind without token should be refused")
	}
	// Non-loopback with a token is allowed.
	t.Setenv(apiTokenEnv, "s3cret")
	if err := RequireBindAllowed("0.0.0.0"); err != nil {
		t.Errorf("non-loopback bind with token should be allowed, got %v", err)
	}
}

func TestRequiresAuth(t *testing.T) {
	protected := []struct {
		method, path string
	}{
		{"POST", "/api/run"},
		{"POST", "/api/run/stop"},
		{"GET", "/api/browse"},
		{"GET", "/api/workdir"},
		{"POST", "/api/chat/start"},
		{"GET", "/api/chat"},
		{"GET", "/api/chat/abc123"},
		{"POST", "/api/chat/abc123/stop"},
		{"POST", "/api/chat/abc123/fork"},
		{"GET", "/api/chat/abc123/tree"},
		{"GET", "/ws/chat/abc123"},
		{"POST", "/api/hub/debug"},
		{"GET", "/api/hub/sessions"},
		{"GET", "/api/hub/sessions/events"},
		{"GET", "/api/sessions/live"},
		{"GET", "/api/runtime/sessions"},
		{"POST", "/api/configs/upload"},
		{"POST", "/api/configs/upload-zip"},
		{"POST", "/api/configs/inline"},
		{"POST", "/api/debug/params"},
		{"GET", "/api/debug/state"},
		{"DELETE", "/api/configs/abc123"},
		{"POST", "/mcp"},          // MCP executes arbitrary registered tools, no auth of its own
		{"POST", "/mcp/anything"}, // standalone --mcp-port server has nothing else mounted on it
		{"POST", "/a2a"},          // same shape as MCP: full tool-execution surface
		{"GET", "/api/providers/models"},     // outbound-request proxy, SSRF-adjacent
		{"GET", "/api/providers/model-info"}, // same endpoint shape as above
	}
	for _, c := range protected {
		r := httptest.NewRequest(c.method, c.path, nil)
		if !requiresAuth(r) {
			t.Errorf("requiresAuth(%s %s) = false, want true", c.method, c.path)
		}
	}

	open := []struct {
		method, path string
	}{
		{"GET", "/"},
		{"GET", "/health"},
		{"GET", "/api/status"},
		{"GET", "/api/sessions"},
		{"GET", "/api/configs"},
		{"GET", "/events"},
		{"GET", "/api/configs/abc123"},      // read of a config is not gated
		{"POST", "/api/hub/register"},       // CLI-report endpoints: the hub forwarder
		{"POST", "/api/hub/deregister"},     // sends no Authorization header at all, so
		{"POST", "/api/hub/ingest"},         // gating these would break CLI<->hub
		{"GET", "/api/hub/commands"},        // reporting whenever a token is set. Only
		{"POST", "/api/hub/message-result"}, // the UI-facing hub endpoints are gated.
	}
	for _, c := range open {
		r := httptest.NewRequest(c.method, c.path, nil)
		if requiresAuth(r) {
			t.Errorf("requiresAuth(%s %s) = true, want false", c.method, c.path)
		}
	}
}

func TestAuthMiddleware(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("no token configured is pass-through", func(t *testing.T) {
		t.Setenv(apiTokenEnv, "")
		rec := httptest.NewRecorder()
		AuthMiddleware(ok).ServeHTTP(rec, httptest.NewRequest("POST", "/api/run", nil))
		if rec.Code != http.StatusOK {
			t.Errorf("with no token, protected endpoint = %d, want 200", rec.Code)
		}
	})

	t.Run("token set, protected endpoint without header is 401", func(t *testing.T) {
		t.Setenv(apiTokenEnv, "s3cret")
		rec := httptest.NewRecorder()
		AuthMiddleware(ok).ServeHTTP(rec, httptest.NewRequest("POST", "/api/run", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("protected endpoint without token = %d, want 401", rec.Code)
		}
	})

	t.Run("token set, wrong header is 401", func(t *testing.T) {
		t.Setenv(apiTokenEnv, "s3cret")
		req := httptest.NewRequest("POST", "/api/run", nil)
		req.Header.Set("Authorization", "Bearer wrong")
		rec := httptest.NewRecorder()
		AuthMiddleware(ok).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("protected endpoint with wrong token = %d, want 401", rec.Code)
		}
	})

	t.Run("token set, correct header passes", func(t *testing.T) {
		t.Setenv(apiTokenEnv, "s3cret")
		req := httptest.NewRequest("POST", "/api/run", nil)
		req.Header.Set("Authorization", "Bearer s3cret")
		rec := httptest.NewRecorder()
		AuthMiddleware(ok).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("protected endpoint with correct token = %d, want 200", rec.Code)
		}
	})

	t.Run("token set, unprotected endpoint stays open", func(t *testing.T) {
		t.Setenv(apiTokenEnv, "s3cret")
		rec := httptest.NewRecorder()
		AuthMiddleware(ok).ServeHTTP(rec, httptest.NewRequest("GET", "/api/status", nil))
		if rec.Code != http.StatusOK {
			t.Errorf("unprotected endpoint with token set = %d, want 200", rec.Code)
		}
	})
}

func TestIsAllowedOrigin(t *testing.T) {
	allowed := []string{
		"",
		"http://localhost",
		"http://localhost:9100",
		"https://localhost:9100",
		"http://127.0.0.1:5173",
		"http://[::1]:9100",
	}
	for _, o := range allowed {
		if !isAllowedOrigin(o) {
			t.Errorf("isAllowedOrigin(%q) = false, want true", o)
		}
	}
	denied := []string{
		"http://evil.com",
		"https://attacker.example",
		"http://192.168.1.99:9100",
		"http://localhost.evil.com",
	}
	for _, o := range denied {
		if isAllowedOrigin(o) {
			t.Errorf("isAllowedOrigin(%q) = true, want false", o)
		}
	}
}
