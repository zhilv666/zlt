package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// clock is an injectable wall clock for deterministic idle-expiry tests.
type clock struct{ t time.Time }

func newClock() *clock {
	return &clock{t: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)}
}

func (c *clock) now() time.Time { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func tempKeyPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "auth.key")
}

func tempSessionPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "auth.db")
}

func mustService(t *testing.T, cfg Config) *Service {
	s, err := NewService(cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return s
}

// ---- Key management ----

func TestLoadOrCreateKey_GeneratesAndPersists(t *testing.T) {
	path := tempKeyPath(t)
	k1, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if len(k1) != keySize {
		t.Fatalf("key length: %d", len(k1))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("key file not created: %v", err)
	}
	// A second call must return the same key (no silent regeneration).
	k2, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if string(k1) != string(k2) {
		t.Fatal("key changed between calls")
	}
}

func TestLoadOrCreateKey_CorruptFileErrors(t *testing.T) {
	path := tempKeyPath(t)
	if err := os.WriteFile(path, []byte("not-valid-base64!@#"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateKey(path); err == nil {
		t.Fatal("expected error for corrupt key file, got nil")
	}
}

func TestLoadOrCreateKey_WrongLengthErrors(t *testing.T) {
	path := tempKeyPath(t)
	// Valid base64url but only 16 bytes — must not be silently accepted.
	if err := os.WriteFile(path, []byte(DisplayKey(make([]byte, 16))), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateKey(path); err == nil {
		t.Fatal("expected error for wrong-length key, got nil")
	}
}

func TestVerifyKey(t *testing.T) {
	key := make([]byte, keySize)
	display := DisplayKey(key)
	if !VerifyKey(display, key) {
		t.Fatal("correct key rejected")
	}
	if VerifyKey(display+"x", key) {
		t.Fatal("wrong key accepted")
	}
	if VerifyKey("", key) {
		t.Fatal("empty key accepted")
	}
}

// ---- Session store ----

func TestSessionLifecycle(t *testing.T) {
	clk := newClock()
	store, err := NewSessionStore(tempSessionPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	fp := "fp-current"
	token, csrf, expires, err := store.Create(fp, true, clk.now())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if token == "" || csrf == "" {
		t.Fatal("empty token or csrf")
	}
	if !expires.Equal(clk.now().Add(SessionMaxIdle)) {
		t.Fatalf("expiry not now+7d: %v", expires)
	}

	// Live lookup.
	sess, ok := store.Lookup(hashToken(token), fp, clk.now())
	if !ok {
		t.Fatal("lookup failed on a fresh session")
	}
	if sess.CSRFToken != csrf || !sess.Remember {
		t.Fatalf("lookup returned wrong session: %+v", sess)
	}

	// Touch extends the idle window.
	clk.advance(SessionMaxIdle - time.Hour)
	newExpires, err := store.Touch(hashToken(token), clk.now())
	if err != nil {
		t.Fatalf("touch: %v", err)
	}
	if !newExpires.Equal(clk.now().Add(SessionMaxIdle)) {
		t.Fatalf("touch did not recompute expiry: %v", newExpires)
	}

	// An expired session (past the new expiry) is no longer live.
	clk.advance(SessionMaxIdle + time.Second)
	if _, ok := store.Lookup(hashToken(token), fp, clk.now()); ok {
		t.Fatal("expired session still live")
	}

	// Purge removes expired rows.
	n, err := store.PurgeExpired(clk.now())
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Fatalf("purge removed %d rows, want 1", n)
	}
}

// Rotating the key (new fingerprint) invalidates every existing session.
func TestSessionKeyFingerprintInvalidation(t *testing.T) {
	clk := newClock()
	store, _ := NewSessionStore(tempSessionPath(t))
	defer store.Close()

	token, _, _, _ := store.Create("old-fp", true, clk.now())
	if _, ok := store.Lookup(hashToken(token), "new-fp", clk.now()); ok {
		t.Fatal("session with old fingerprint still valid after rotation")
	}
	if err := store.DeleteByKeyFingerprint("old-fp"); err != nil {
		t.Fatalf("delete by fingerprint: %v", err)
	}
}

// ---- Service: login flow ----

// authTestMux wires the four auth endpoints plus a single protected route,
// wrapped by the middleware, mirroring how the real server uses the Service.
func authTestMux(t *testing.T, s *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", s.HandleLogin)
	mux.HandleFunc("/api/auth/session", s.HandleSession)
	mux.HandleFunc("/api/auth/touch", s.HandleTouch)
	mux.HandleFunc("/api/auth/logout", s.HandleLogout)
	mux.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		writeAuthJSON(w, http.StatusOK, 0, "ok", map[string]string{"route": "tasks"})
	})
	return s.Middleware(mux)
}

func newTestService(t *testing.T, publicURL string) (*Service, []byte, *clock) {
	clk := newClock()
	key, err := LoadOrCreateKey(tempKeyPath(t))
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewSessionStore(tempSessionPath(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	s := mustService(t, Config{
		Key:       key,
		Sessions:  store,
		PublicURL: publicURL,
		Now:       clk.now,
	})
	return s, key, clk
}

// doJSON issues a request with optional JSON body and returns status + decoded body.
func doJSON(t *testing.T, h http.Handler, method, path, body, cookie, csrf string) (int, map[string]interface{}) {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Origin", "https://zlt.test")
	req.Host = "zlt.test"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.Header.Set("Cookie", CookieName+"="+cookie)
	}
	if csrf != "" {
		req.Header.Set(CSRFHeader, csrf)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var data map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &data)
	return rec.Code, data
}

func extractCookie(rec *httptest.ResponseRecorder) string {
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			return c.Value
		}
	}
	return ""
}

// dataOf digs the response body's nested "data" object out of the standard
// {code,msg,data} envelope that writeAuthJSON produces.
func dataOf(body map[string]interface{}) map[string]interface{} {
	if d, ok := body["data"].(map[string]interface{}); ok {
		return d
	}
	return map[string]interface{}{}
}

func TestLoginFlow(t *testing.T) {
	s, key, clk := newTestService(t, "")
	clk.advance(0)
	h := authTestMux(t, s)
	display := DisplayKey(key)

	// Correct login (same-origin).
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"key":"`+display+`","remember":true}`))
	req.Header.Set("Origin", "https://zlt.test")
	req.Host = "zlt.test"
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login status: %d", rec.Code)
	}
	cookie := extractCookie(rec)
	if cookie == "" {
		t.Fatal("login did not set cookie")
	}
	csrf, _ := decodeLoginBody(rec.Body.String())
	if csrf == "" {
		t.Fatal("login did not return csrf token")
	}

	// Session status with the cookie.
	code, data := doJSON(t, h, "GET", "/api/auth/session", "", cookie, "")
	if code != 200 || dataOf(data)["authenticated"] != true {
		t.Fatalf("session check: %d %v", code, data)
	}

	// Touch requires CSRF; without it → 403.
	code, _ = doJSON(t, h, "POST", "/api/auth/touch", "", cookie, "")
	if code != 403 {
		t.Fatalf("touch without csrf: %d, want 403", code)
	}
	// With CSRF → 200 and refreshed cookie.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/auth/touch", nil)
	req2.Header.Set("Origin", "https://zlt.test")
	req2.Host = "zlt.test"
	req2.Header.Set("Cookie", CookieName+"="+cookie)
	req2.Header.Set(CSRFHeader, csrf)
	h.ServeHTTP(rec2, req2)
	if rec2.Code != 200 {
		t.Fatalf("touch status: %d", rec2.Code)
	}

	// Logout with CSRF.
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("POST", "/api/auth/logout", nil)
	req3.Header.Set("Origin", "https://zlt.test")
	req3.Host = "zlt.test"
	req3.Header.Set("Cookie", CookieName+"="+cookie)
	req3.Header.Set(CSRFHeader, csrf)
	h.ServeHTTP(rec3, req3)
	if rec3.Code != 200 {
		t.Fatalf("logout status: %d", rec3.Code)
	}
	// After logout, the session is gone.
	code, data = doJSON(t, h, "GET", "/api/auth/session", "", cookie, "")
	if dataOf(data)["authenticated"] == true {
		t.Fatalf("session still authenticated after logout: %v", data)
	}
}

func decodeLoginBody(body string) (string, error) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return "", err
	}
	if d, ok := m["data"].(map[string]interface{}); ok {
		if c, ok := d["csrf_token"].(string); ok {
			return c, nil
		}
	}
	return "", nil
}

func TestLoginWrongKeyAndRateLimit(t *testing.T) {
	s, _, _ := newTestService(t, "")
	h := authTestMux(t, s)

	for i := 0; i < 5; i++ {
		code, _ := doJSON(t, h, "POST", "/api/auth/login", `{"key":"wrong","remember":false}`, "", "")
		if code != 401 {
			t.Fatalf("attempt %d: status %d, want 401", i, code)
		}
	}
	// 6th failure within the window → 429.
	code, _ := doJSON(t, h, "POST", "/api/auth/login", `{"key":"wrong","remember":false}`, "", "")
	if code != 429 {
		t.Fatalf("6th attempt: status %d, want 429", code)
	}
}

func TestLoginCrossOrigin(t *testing.T) {
	s, key, _ := newTestService(t, "")
	h := authTestMux(t, s)
	display := DisplayKey(key)

	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"key":"`+display+`","remember":false}`))
	req.Header.Set("Origin", "https://evil.test")
	req.Host = "zlt.test"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("cross-origin login: %d, want 403", rec.Code)
	}
}

// ---- Middleware ----

func TestMiddlewareAnonymousBlocked(t *testing.T) {
	s, _, _ := newTestService(t, "")
	h := authTestMux(t, s)
	code, _ := doJSON(t, h, "GET", "/api/tasks", "", "", "")
	if code != 401 {
		t.Fatalf("anonymous business request: %d, want 401", code)
	}
}

func TestMiddlewareStateChangingRequiresCSRF(t *testing.T) {
	s, key, _ := newTestService(t, "")
	h := authTestMux(t, s)
	display := DisplayKey(key)

	// Log in to get a cookie + csrf.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"key":"`+display+`","remember":true}`))
	req.Header.Set("Origin", "https://zlt.test")
	req.Host = "zlt.test"
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	cookie := extractCookie(rec)
	csrf, _ := decodeLoginBody(rec.Body.String())

	// POST /api/tasks without CSRF → 403.
	code, _ := doJSON(t, h, "POST", "/api/tasks", `{}`, cookie, "")
	if code != 403 {
		t.Fatalf("post without csrf: %d, want 403", code)
	}
	// With CSRF → 200.
	code, _ = doJSON(t, h, "POST", "/api/tasks", `{}`, cookie, csrf)
	if code != 200 {
		t.Fatalf("post with csrf: %d, want 200", code)
	}
	// GET /api/tasks with cookie → 200 (no csrf needed).
	code, _ = doJSON(t, h, "GET", "/api/tasks", "", cookie, "")
	if code != 200 {
		t.Fatalf("get with cookie: %d, want 200", code)
	}
}

// Idle 7-day boundary: a session untouched past the window cannot be revived
// by a touch, and the middleware treats it as anonymous (401).
func TestSessionIdleExpiry(t *testing.T) {
	s, key, clk := newTestService(t, "")
	h := authTestMux(t, s)
	display := DisplayKey(key)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"key":"`+display+`","remember":true}`))
	req.Header.Set("Origin", "https://zlt.test")
	req.Host = "zlt.test"
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	cookie := extractCookie(rec)
	csrf, _ := decodeLoginBody(rec.Body.String())

	// Advance past the idle window. No activity in between → no touch.
	clk.advance(SessionMaxIdle + time.Second)

	// Middleware now rejects the stale session as anonymous.
	code, _ := doJSON(t, h, "GET", "/api/tasks", "", cookie, "")
	if code != 401 {
		t.Fatalf("stale session: %d, want 401", code)
	}
	// Touch cannot revive an expired session.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/auth/touch", nil)
	req2.Header.Set("Origin", "https://zlt.test")
	req2.Host = "zlt.test"
	req2.Header.Set("Cookie", CookieName+"="+cookie)
	req2.Header.Set(CSRFHeader, csrf)
	h.ServeHTTP(rec2, req2)
	if rec2.Code != 401 {
		t.Fatalf("touch on expired session: %d, want 401", rec2.Code)
	}
}

// ---- Public URL validation ----

func TestPublicURLValidation(t *testing.T) {
	key := make([]byte, keySize)
	store, err := NewSessionStore(tempSessionPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cases := []struct {
		url  string
		ok   bool
	}{
		{"", true},                         // local HTTP: no public URL
		{"https://zlt.example", true},      // valid https root
		{"http://zlt.example", false},      // not https
		{"https://zlt.example/admin", false}, // path not allowed
		{"https://user:pass@zlt.example", false}, // userinfo not allowed
		{"https://zlt.example?x=1", false}, // query not allowed
		{"https://zlt.example#/frag", false}, // fragment not allowed
		{"not a url", false},
	}
	for _, c := range cases {
		_, err := NewService(Config{Key: key, Sessions: store, PublicURL: c.url})
		if c.ok && err != nil {
			t.Errorf("url %q: expected ok, got %v", c.url, err)
		}
		if !c.ok && err == nil {
			t.Errorf("url %q: expected error, got ok", c.url)
		}
	}
}
