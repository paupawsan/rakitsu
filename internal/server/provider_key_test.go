package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderAPIKeyFromRequest_PrefersAuthHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/providers/models?base_url=http://x&api_key=from-query", nil)
	r.Header.Set("Authorization", "Bearer from-header")

	if got := providerAPIKeyFromRequest(r); got != "from-header" {
		t.Fatalf("providerAPIKeyFromRequest() = %q, want %q (header must win over query)", got, "from-header")
	}
}

func TestProviderAPIKeyFromRequest_FallsBackToQuery(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/providers/models?base_url=http://x&api_key=from-query", nil)

	if got := providerAPIKeyFromRequest(r); got != "from-query" {
		t.Fatalf("providerAPIKeyFromRequest() = %q, want %q (must fall back to query when no header)", got, "from-query")
	}
}

func TestProviderAPIKeyFromRequest_IgnoresNonBearerAuthHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/providers/models?base_url=http://x&api_key=from-query", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	if got := providerAPIKeyFromRequest(r); got != "from-query" {
		t.Fatalf("providerAPIKeyFromRequest() = %q, want %q (non-Bearer header must not shadow the query fallback)", got, "from-query")
	}
}

func TestProviderAPIKeyFromRequest_EmptyWhenNeitherSet(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/providers/models?base_url=http://x", nil)

	if got := providerAPIKeyFromRequest(r); got != "" {
		t.Fatalf("providerAPIKeyFromRequest() = %q, want empty", got)
	}
}
