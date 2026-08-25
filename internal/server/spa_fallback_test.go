package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSpaFallback_ServesRealFileAsIs regression-guards the happy path: a
// request for a file that actually exists in fsys must be served normally,
// not redirected to index.html.
func TestSpaFallback_ServesRealFileAsIs(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "index.html", "<html>shell</html>")
	mustWrite(t, dir, "app.js", "console.log(1)")

	fsys := http.Dir(dir)
	handler := spaFallback(fsys, http.FileServer(fsys))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "console.log") {
		t.Fatalf("real file body = %q, want the actual app.js content", rec.Body.String())
	}
}

// TestSpaFallback_ServesIndexForClientRoute regression-guards the actual
// bug: spaFallback's own comment claims a 404 falls back to index.html, but
// the body was a bare passthrough — a client-side route with no matching
// file (e.g. /sessions/abc123, a Vue Router path that only exists in JS)
// got a raw 404 from the embedded file server instead of the SPA shell.
func TestSpaFallback_ServesIndexForClientRoute(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "index.html", "<html>shell</html>")

	fsys := http.Dir(dir)
	handler := spaFallback(fsys, http.FileServer(fsys))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sessions/abc123", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("client-route status = %d, want 200 (index.html fallback)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "shell") {
		t.Fatalf("client route did not serve index.html shell, got %q", rec.Body.String())
	}
}

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
