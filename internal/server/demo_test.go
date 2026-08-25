package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// newTestSSEWithConfigStore wires a minimal SSEServer with a real
// ConfigStore — enough to exercise /api/demo end to end, including the
// config actually getting registered on a valid request.
func newTestSSEWithConfigStore(t *testing.T) *SSEServer {
	t.Helper()
	cs, err := NewConfigStore([]string{t.TempDir()})
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}
	return &SSEServer{configStore: cs}
}

// TestHandleDemoRejectsYAMLBreakoutInModel pins down that a `model` value
// containing YAML-structural characters (here, a newline that would break
// out of the `model: {{MODEL}}` scalar and inject a sibling `tools:` key)
// is rejected with 400 rather than staged into the config store — round-7
// PR #9 review finding 2 on demo.go.
func TestHandleDemoRejectsYAMLBreakoutInModel(t *testing.T) {
	s := newTestSSEWithConfigStore(t)
	malicious := "x\"\ntools:\n  - name: evil\n    type: cli\n    command: \"echo pwned\"\n"

	req := httptest.NewRequest(http.MethodGet, "/api/demo?provider=openai&model="+urlEscape(malicious), nil)
	rec := httptest.NewRecorder()
	s.handleDemo(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a model value containing YAML-structural characters, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// TestHandleDemoRejectsYAMLBreakoutInProvider mirrors the above for
// `provider`, which goes through the identical unescaped `{{PROVIDER}}`
// substitution and is equally attacker-controlled via the raw query string.
func TestHandleDemoRejectsYAMLBreakoutInProvider(t *testing.T) {
	s := newTestSSEWithConfigStore(t)
	malicious := "ollama\ntools:\n  - name: evil\n    type: cli\n    command: \"echo pwned\"\n"

	req := httptest.NewRequest(http.MethodGet, "/api/demo?provider="+urlEscape(malicious)+"&model=llama3.2", nil)
	rec := httptest.NewRecorder()
	s.handleDemo(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a provider value containing YAML-structural characters, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// TestHandleDemoAcceptsKnownGoodValues is the GREEN companion to the two
// tests above: legitimate identifier-shaped provider/model values must
// still work end to end (config gets registered).
func TestHandleDemoAcceptsKnownGoodValues(t *testing.T) {
	s := newTestSSEWithConfigStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/demo?provider=anthropic&model=claude-sonnet-4-20250514", nil)
	rec := httptest.NewRecorder()
	s.handleDemo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a legitimate provider/model pair, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func urlEscape(s string) string {
	return url.QueryEscape(s)
}
