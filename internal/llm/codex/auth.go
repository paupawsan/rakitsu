package codex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Package-level knobs so tests can point refresh at an httptest server.
var (
	// refreshURL is the OAuth token endpoint used for refresh_token grants.
	refreshURL = "https://auth.openai.com/oauth/token"
	// clientID is the public OAuth client id Codex CLI registers with.
	clientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	// refreshWindow: refresh proactively when the access token expires within it.
	refreshWindow = 5 * time.Minute
	// now is swappable in tests.
	now = time.Now
)

// DefaultAuthFile is where Codex CLI stores its login.
const DefaultAuthFile = "~/.codex/auth.json"

// authFile mirrors the on-disk auth.json written by Codex CLI. Unknown
// fields are preserved through a load/save cycle via Extra.
type authFile struct {
	AuthMode     string     `json:"auth_mode,omitempty"`
	OpenAIAPIKey *string    `json:"OPENAI_API_KEY"`
	Tokens       *tokenData `json:"tokens,omitempty"`
	LastRefresh  string     `json:"last_refresh,omitempty"`

	extra map[string]json.RawMessage
}

type tokenData struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id,omitempty"`
}

// tokenSource loads, refreshes and persists the Codex login. Safe for
// concurrent use by many agents in one process.
// It owns no HTTP client: every caller passes its own, so two providers on
// the same file each refresh through their own transport/timeout settings.
type tokenSource struct {
	path string

	mu   sync.Mutex
	auth *authFile
	// loaded is the SHA-256 of the file bytes auth was parsed from, so a
	// file replaced while the process runs (logout/login, account switch)
	// is picked up on the next call instead of serving the old account.
	// Content, not mtime/size: a same-sized rewrite with a preserved
	// timestamp must still be noticed. The file is a few KB; reading it
	// per call is cheaper than one wrong-account request.
	loaded [sha256.Size]byte
}

// ensureCurrent loads auth.json on first use and reloads it whenever the
// bytes on disk differ from what was last parsed.
func (ts *tokenSource) ensureCurrent() error {
	raw, err := os.ReadFile(ts.path)
	if err != nil {
		return readErr(ts.path, err)
	}
	if ts.auth != nil && sha256.Sum256(raw) == ts.loaded {
		return nil
	}
	return ts.parse(raw)
}

func readErr(path string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("codex: no login found at %s — run \"codex login\" first", path)
	}
	return fmt.Errorf("codex: read %s: %w", path, err)
}

// expandHome replaces a leading "~" with the user's home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// sources holds one tokenSource per resolved auth-file path for the whole
// process. Every Provider and ListModels call for the same file shares it,
// so a refresh of the rotating refresh token is serialised process-wide
// instead of per instance: two agents refreshing at once would otherwise
// invalidate each other's refresh token.
var sources = struct {
	sync.Mutex
	m map[string]*tokenSource
}{m: map[string]*tokenSource{}}

func newTokenSource(path string) *tokenSource {
	if path == "" {
		path = DefaultAuthFile
	}
	path = expandHome(path)
	sources.Lock()
	defer sources.Unlock()
	if ts, ok := sources.m[path]; ok {
		return ts
	}
	ts := &tokenSource{path: path}
	sources.m[path] = ts
	return ts
}

func (ts *tokenSource) load() error {
	raw, err := os.ReadFile(ts.path)
	if err != nil {
		return readErr(ts.path, err)
	}
	return ts.parse(raw)
}

// parse replaces the in-memory auth with the given file bytes.
func (ts *tokenSource) parse(raw []byte) error {
	var af authFile
	if err := json.Unmarshal(raw, &af); err != nil {
		return fmt.Errorf("codex: parse %s: %w", ts.path, err)
	}
	if err := json.Unmarshal(raw, &af.extra); err != nil {
		return fmt.Errorf("codex: parse %s: %w", ts.path, err)
	}
	for _, k := range []string{"auth_mode", "OPENAI_API_KEY", "tokens", "last_refresh"} {
		delete(af.extra, k)
	}
	if af.Tokens == nil || af.Tokens.AccessToken == "" || af.Tokens.RefreshToken == "" {
		return fmt.Errorf("codex: %s has no ChatGPT tokens — run \"codex login\" (not an API-key login)", ts.path)
	}
	if af.Tokens.AccountID == "" {
		af.Tokens.AccountID = accountIDFromJWT(af.Tokens.IDToken)
	}
	ts.auth = &af
	ts.loaded = sha256.Sum256(raw)
	return nil
}

// save writes auth.json atomically with mode 0600.
func (ts *tokenSource) save() error {
	out := map[string]json.RawMessage{}
	for k, v := range ts.auth.extra {
		out[k] = v
	}
	put := func(k string, v interface{}) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		out[k] = b
		return nil
	}
	if ts.auth.AuthMode != "" {
		if err := put("auth_mode", ts.auth.AuthMode); err != nil {
			return err
		}
	}
	if err := put("OPENAI_API_KEY", ts.auth.OpenAIAPIKey); err != nil {
		return err
	}
	if err := put("tokens", ts.auth.Tokens); err != nil {
		return err
	}
	if err := put("last_refresh", ts.auth.LastRefresh); err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	// Unique temp name per writer: a fixed ".tmp" sibling would let two
	// writers (this process and Codex CLI, say) clobber each other's
	// half-written file or race on the rename.
	f, err := os.CreateTemp(filepath.Dir(ts.path), filepath.Base(ts.path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("codex: create temp for %s: %w", ts.path, err)
	}
	tmp := f.Name()
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("codex: chmod %s: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("codex: write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("codex: close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, ts.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("codex: replace %s: %w", ts.path, err)
	}
	ts.loaded = sha256.Sum256(data)
	return nil
}

// token returns a valid access token and account id, refreshing first when
// the token is expired or about to expire.
func (ts *tokenSource) token(ctx context.Context, client *http.Client) (access, accountID string, err error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if err := ts.ensureCurrent(); err != nil {
		return "", "", err
	}
	if exp, ok := jwtExpiry(ts.auth.Tokens.AccessToken); ok && now().Add(refreshWindow).After(exp) {
		if err := ts.refreshLocked(ctx, client); err != nil {
			return "", "", err
		}
	}
	return ts.auth.Tokens.AccessToken, ts.auth.Tokens.AccountID, nil
}

// forceRefresh is called after a 401. stale is the token that was rejected;
// if another goroutine already refreshed past it, no second refresh is made.
func (ts *tokenSource) forceRefresh(ctx context.Context, client *http.Client, stale string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if err := ts.ensureCurrent(); err != nil {
		return err
	}
	if ts.auth.Tokens.AccessToken != stale {
		return nil
	}
	return ts.refreshLocked(ctx, client)
}

// adoptFresherOnDisk re-reads auth.json and, when another writer (Codex CLI
// or another process) already rotated the token and the on-disk access
// token is not about to expire, takes those tokens instead of refreshing a
// second time. Reports whether it did so.
func (ts *tokenSource) adoptFresherOnDisk() bool {
	raw, err := os.ReadFile(ts.path)
	if err != nil {
		return false
	}
	var af authFile
	if err := json.Unmarshal(raw, &af); err != nil || af.Tokens == nil {
		return false
	}
	if af.Tokens.AccessToken == "" || af.Tokens.AccessToken == ts.auth.Tokens.AccessToken {
		return false
	}
	exp, ok := jwtExpiry(af.Tokens.AccessToken)
	if !ok || now().Add(refreshWindow).After(exp) {
		return false
	}
	ts.auth.Tokens.AccessToken = af.Tokens.AccessToken
	if af.Tokens.RefreshToken != "" {
		ts.auth.Tokens.RefreshToken = af.Tokens.RefreshToken
	}
	if af.Tokens.IDToken != "" {
		ts.auth.Tokens.IDToken = af.Tokens.IDToken
	}
	if af.Tokens.AccountID != "" {
		ts.auth.Tokens.AccountID = af.Tokens.AccountID
	}
	if af.LastRefresh != "" {
		ts.auth.LastRefresh = af.LastRefresh
	}
	ts.loaded = sha256.Sum256(raw)
	return true
}

// refreshLocked runs with ts.mu held (in-process exclusion) and takes the
// auth file's lock file as well (cross-process exclusion, e.g. two rakitsu
// processes sharing one login) so the read-check-refresh-write sequence is
// atomic against other rakitsu processes: the re-read happens inside the
// lock, so a process that lost the race adopts the winner's token instead
// of spending the already-consumed refresh token.
func (ts *tokenSource) refreshLocked(ctx context.Context, client *http.Client) error {
	unlock, err := lockFile(ts.path + ".lock")
	if err != nil {
		return fmt.Errorf("codex: lock %s: %w", ts.path+".lock", err)
	}
	defer unlock()
	if ts.adoptFresherOnDisk() {
		return nil
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	body, _ := json.Marshal(map[string]string{
		"client_id":     clientID,
		"grant_type":    "refresh_token",
		"refresh_token": ts.auth.Tokens.RefreshToken,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("codex: build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("codex: token refresh: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return newStatusError(resp.StatusCode, "codex: token refresh failed (HTTP %d) — run \"codex login\" again", resp.StatusCode)
	}
	var out struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("codex: decode refresh response: %w", err)
	}
	if out.AccessToken == "" {
		return errors.New("codex: token refresh returned no access_token — run \"codex login\" again")
	}
	ts.auth.Tokens.AccessToken = out.AccessToken
	if out.IDToken != "" {
		ts.auth.Tokens.IDToken = out.IDToken
	}
	if out.RefreshToken != "" {
		ts.auth.Tokens.RefreshToken = out.RefreshToken
	}
	if ts.auth.Tokens.AccountID == "" {
		ts.auth.Tokens.AccountID = accountIDFromJWT(ts.auth.Tokens.IDToken)
	}
	ts.auth.LastRefresh = now().UTC().Format(time.RFC3339Nano)
	if err := ts.save(); err != nil {
		// The in-memory token is still good; persisting is best-effort.
		return nil
	}
	return nil
}

// jwtClaims decodes the payload of a JWT without verifying it.
func jwtClaims(token string) (map[string]interface{}, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil, false
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, false
	}
	return claims, true
}

func jwtExpiry(token string) (time.Time, bool) {
	claims, ok := jwtClaims(token)
	if !ok {
		return time.Time{}, false
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(int64(exp), 0), true
}

// accountIDFromJWT reads the ChatGPT account id claim Codex CLI relies on.
func accountIDFromJWT(idToken string) string {
	claims, ok := jwtClaims(idToken)
	if !ok {
		return ""
	}
	auth, _ := claims["https://api.openai.com/auth"].(map[string]interface{})
	id, _ := auth["chatgpt_account_id"].(string)
	return id
}
