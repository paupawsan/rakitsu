package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeJWT builds an unsigned JWT with the given claims.
func fakeJWT(t *testing.T, claims map[string]interface{}) string {
	t.Helper()
	enc := func(v interface{}) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return enc(map[string]string{"alg": "none"}) + "." + enc(claims) + ".sig"
}

func writeAuth(t *testing.T, dir string, access, refresh, idTok, account string) string {
	t.Helper()
	path := filepath.Join(dir, "auth.json")
	doc := map[string]interface{}{
		"auth_mode":      "chatgpt",
		"OPENAI_API_KEY": nil,
		"tokens": map[string]string{
			"id_token": idTok, "access_token": access, "refresh_token": refresh, "account_id": account,
		},
		"last_refresh": "2026-09-01T00:00:00Z",
		"future_field": map[string]int{"keep": 1},
	}
	b, _ := json.Marshal(doc)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTokenSource_MissingFile(t *testing.T) {
	ts := newTokenSource(filepath.Join(t.TempDir(), "nope.json"))
	_, _, err := ts.token(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), `run "codex login"`) {
		t.Fatalf("want codex login hint, got %v", err)
	}
}

func TestTokenSource_AccountIDFromIDToken(t *testing.T) {
	future := time.Now().Add(time.Hour).Unix()
	access := fakeJWT(t, map[string]interface{}{"exp": future})
	id := fakeJWT(t, map[string]interface{}{"https://api.openai.com/auth": map[string]interface{}{"chatgpt_account_id": "acct-123"}})
	path := writeAuth(t, t.TempDir(), access, "r1", id, "")
	ts := newTokenSource(path)
	tok, acct, err := ts.token(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if tok != access || acct != "acct-123" {
		t.Fatalf("got token=%q acct=%q", tok, acct)
	}
}

func TestTokenSource_RefreshesWhenExpiringAndPersists(t *testing.T) {
	old := fakeJWT(t, map[string]interface{}{"exp": time.Now().Add(time.Minute).Unix()})
	fresh := fakeJWT(t, map[string]interface{}{"exp": time.Now().Add(time.Hour).Unix()})

	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": fresh, "refresh_token": "r2", "id_token": "x.y.z"})
	}))
	defer srv.Close()
	origURL := refreshURL
	refreshURL = srv.URL
	defer func() { refreshURL = origURL }()

	path := writeAuth(t, t.TempDir(), old, "r1", "", "acct-1")
	ts := newTokenSource(path)
	tok, _, err := ts.token(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if tok != fresh {
		t.Fatalf("expected refreshed token")
	}
	if gotBody["grant_type"] != "refresh_token" || gotBody["refresh_token"] != "r1" || gotBody["client_id"] != clientID {
		t.Fatalf("bad refresh body: %v", gotBody)
	}

	// Persisted, mode 0600, unknown fields kept.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	_ = json.Unmarshal(raw, &doc)
	tokens := doc["tokens"].(map[string]interface{})
	if tokens["access_token"] != fresh || tokens["refresh_token"] != "r2" || tokens["account_id"] != "acct-1" {
		t.Fatalf("persisted tokens wrong: %v", tokens)
	}
	if _, ok := doc["future_field"]; !ok {
		t.Fatalf("unknown field dropped on save")
	}
	if doc["last_refresh"] == "2026-09-01T00:00:00Z" {
		t.Fatalf("last_refresh not updated")
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", st.Mode().Perm())
	}
	if leftovers, _ := filepath.Glob(path + ".*.tmp"); len(leftovers) != 0 {
		t.Fatalf("temp file left behind: %v", leftovers)
	}
}

// Two providers on the same auth file must share one token source, so an
// expiring token is refreshed exactly once even when both ask at the same
// time; a per-instance mutex would let them rotate the refresh token twice.
func TestTokenSource_SharedPerPath_RefreshesOnce(t *testing.T) {
	old := fakeJWT(t, map[string]interface{}{"exp": time.Now().Add(time.Minute).Unix()})
	fresh := fakeJWT(t, map[string]interface{}{"exp": time.Now().Add(time.Hour).Unix()})
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond) // widen the window two refreshes could overlap in
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": fresh, "refresh_token": "r2"})
	}))
	defer srv.Close()
	origURL := refreshURL
	refreshURL = srv.URL
	defer func() { refreshURL = origURL }()

	path := writeAuth(t, t.TempDir(), old, "r1", "", "acct-1")
	a := newTokenSource(path)
	b := newTokenSource(path)
	if a != b {
		t.Fatalf("expected one shared token source per path")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := newTokenSource(path).token(context.Background(), nil); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatalf("refresh called %d times, want 1", calls)
	}
}

// When another process (Codex CLI) already rotated the token on disk, we
// adopt it instead of spending our now-stale refresh token.
func TestTokenSource_AdoptsFresherOnDiskTokenInsteadOfRefreshing(t *testing.T) {
	old := fakeJWT(t, map[string]interface{}{"exp": time.Now().Add(time.Minute).Unix()})
	fresh := fakeJWT(t, map[string]interface{}{"exp": time.Now().Add(time.Hour).Unix()})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("refresh endpoint must not be called")
	}))
	defer srv.Close()
	origURL := refreshURL
	refreshURL = srv.URL
	defer func() { refreshURL = origURL }()

	dir := t.TempDir()
	path := writeAuth(t, dir, old, "r1", "", "acct-1")
	ts := newTokenSource(path)
	if err := ts.load(); err != nil {
		t.Fatal(err)
	}
	// Someone else rotates the file underneath us.
	writeAuth(t, dir, fresh, "r-rotated", "", "acct-1")

	tok, _, err := ts.token(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if tok != fresh || ts.auth.Tokens.RefreshToken != "r-rotated" {
		t.Fatalf("did not adopt on-disk tokens: tok match=%v refresh=%q", tok == fresh, ts.auth.Tokens.RefreshToken)
	}
}

func TestTokenSource_RefreshFailureHintsLogin(t *testing.T) {
	old := fakeJWT(t, map[string]interface{}{"exp": time.Now().Add(-time.Minute).Unix()})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	origURL := refreshURL
	refreshURL = srv.URL
	defer func() { refreshURL = origURL }()

	path := writeAuth(t, t.TempDir(), old, "r1", "", "acct-1")
	ts := newTokenSource(path)
	_, _, err := ts.token(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), `run "codex login" again`) {
		t.Fatalf("want login-again hint, got %v", err)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("want status in message, got %v", err)
	}
}

func TestTokenSource_ForceRefreshSkipsWhenAlreadyRotated(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "new"})
	}))
	defer srv.Close()
	origURL := refreshURL
	refreshURL = srv.URL
	defer func() { refreshURL = origURL }()

	path := writeAuth(t, t.TempDir(), "stale", "r1", "", "acct-1")
	ts := newTokenSource(path)
	if err := ts.forceRefresh(context.Background(), nil, "stale"); err != nil {
		t.Fatal(err)
	}
	if err := ts.forceRefresh(context.Background(), nil, "stale"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("refresh called %d times, want 1", calls)
	}
}

// The lock file must exclude a second holder until the first releases it —
// this is what makes read-check-refresh-write atomic across processes.
func TestLockFile_Excludes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json.lock")
	unlock, err := lockFile(path)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan struct{})
	go func() {
		u2, err := lockFile(path)
		if err != nil {
			t.Error(err)
		}
		close(acquired)
		u2()
	}()
	select {
	case <-acquired:
		if runtime.GOOS != "windows" {
			t.Fatalf("second lock acquired while first still held")
		}
	case <-time.After(150 * time.Millisecond):
	}
	unlock()
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatalf("second lock never acquired after release")
	}
}

// A re-login or account switch rewrites auth.json while the process runs;
// the shared token source must notice and stop serving the old account
// even though the old access token is still valid.
func TestTokenSource_ReloadsWhenFileReplaced(t *testing.T) {
	first := fakeJWT(t, map[string]interface{}{"exp": time.Now().Add(time.Hour).Unix()})
	second := fakeJWT(t, map[string]interface{}{"exp": time.Now().Add(2 * time.Hour).Unix()})
	dir := t.TempDir()
	path := writeAuth(t, dir, first, "r1", "", "acct-old")
	ts := newTokenSource(path)
	tok, acct, err := ts.token(context.Background(), nil)
	if err != nil || tok != first || acct != "acct-old" {
		t.Fatalf("initial: tok match=%v acct=%q err=%v", tok == first, acct, err)
	}
	// Same-sized rewrite with the timestamp put back: only the content
	// changed, so a stat-based check would miss it.
	st, _ := os.Stat(path)
	writeAuth(t, dir, second, "r1", "", "acct-new")
	if err := os.Chtimes(path, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	if st2, _ := os.Stat(path); st2.Size() != st.Size() || !st2.ModTime().Equal(st.ModTime()) {
		t.Fatalf("test setup: rewrite must keep size and mtime (size %d→%d)", st.Size(), st2.Size())
	}
	tok, acct, err = ts.token(context.Background(), nil)
	if err != nil || tok != second || acct != "acct-new" {
		t.Fatalf("after replace: tok match=%v acct=%q err=%v", tok == second, acct, err)
	}
}
