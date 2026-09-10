package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// The codex catalog path must not require base_url, and a missing login must
// come back as a JSON error the UI can show, not a 500.
func TestHandleProviderModels_CodexMissingLogin(t *testing.T) {
	s := &SSEServer{}
	missing := filepath.Join(t.TempDir(), "auth.json")
	req := httptest.NewRequest(http.MethodGet, "/api/providers/models?type=codex&credentials_file="+missing, nil)
	rec := httptest.NewRecorder()
	s.handleProviderModels(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out struct {
		Models []string `json:"models"`
		Error  string   `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Models) != 0 || !strings.Contains(out.Error, "codex login") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
