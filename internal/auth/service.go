package auth

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CookieName is the session cookie name.
const CookieName = "zlt_sess"

// CSRFHeader is the request header carrying the per-session CSRF token.
const CSRFHeader = "X-CSRF-Token"

// Config configures the auth Service.
type Config struct {
	Key            string
	Sessions       *SessionStore
	PublicURL      string   // "" (local HTTP) or a https root, e.g. https://zlt.example
	TrustedProxies []string // additional trusted proxy CIDRs/IPs; loopback is always trusted
	Now            func() time.Time
}

// Service is the entry point for browser authentication. It owns the access
// key, the session store and the login rate limiter, and exposes the unified
// middleware plus the four auth endpoints.
type Service struct {
	key            string
	fingerprint    string
	sessions       *SessionStore
	secureCookies  bool
	publicURL      string
	trustedNets     []*net.IPNet
	limiter        *loginLimiter
	now            func() time.Time
}

// NewService validates the public URL, derives the key fingerprint and the
// trusted-proxy set, and returns a ready Service. An invalid public URL is a
// startup-fatal error rather than a silent downgrade to insecure cookies.
func NewService(cfg Config) (*Service, error) {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	secure := false
	publicURL := strings.TrimSpace(cfg.PublicURL)
	if publicURL != "" {
		u, err := url.Parse(publicURL)
		if err != nil || u.Scheme != "https" {
			return nil, errInvalidPublicURL(publicURL)
		}
		if u.User != nil || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
			return nil, errInvalidPublicURL(publicURL)
		}
		publicURL = strings.TrimRight(u.String(), "/")
		secure = true
	}

	trusted, err := buildTrustedNets(cfg.TrustedProxies)
	if err != nil {
		return nil, err
	}

	return &Service{
		key:           cfg.Key,
		fingerprint:   Fingerprint(cfg.Key),
		sessions:      cfg.Sessions,
		secureCookies: secure,
		publicURL:     publicURL,
		trustedNets:   trusted,
		limiter:       newLoginLimiter(now),
		now:           now,
	}, nil
}

// PublicURL returns the configured https root, or "" when running over local HTTP.
func (s *Service) PublicURL() string { return s.publicURL }

// SecureCookies reports whether the Secure flag is set on cookies (public URL
// is https). Callers that build redirect/dashboard URLs use this.
func (s *Service) SecureCookies() bool { return s.secureCookies }

// PurgeExpired removes stale sessions; call at startup and periodically.
func (s *Service) PurgeExpired() (int64, error) {
	return s.sessions.PurgeExpired(s.now())
}

// Middleware wraps a handler with unified auth. Public paths (the index page,
// login, session status) pass through anonymously; every other route requires
// a live session (401 otherwise) and, for state-changing methods, a valid CSRF
// token (403 otherwise). It never renews a session: renewal is exclusively the
// /api/auth/touch endpoint, so polling and SSE cannot extend idle lifetime.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.isPublic(r) {
			next.ServeHTTP(w, r)
			return
		}
		sess, _, ok := s.SessionFromRequest(r)
		if !ok {
			writeAuthJSON(w, http.StatusUnauthorized, 1, "unauthorized", nil)
			return
		}
		if isStateChanging(r.Method) && !s.csrfOK(r, sess) {
			writeAuthJSON(w, http.StatusForbidden, 1, "csrf token invalid", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SessionFromRequest parses the session cookie and verifies it against the
// store. It returns the session, the raw token (for cookie refresh) and a
// live flag. Read-only: it does not extend the session.
func (s *Service) SessionFromRequest(r *http.Request) (Session, string, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return Session{}, "", false
	}
	sess, ok := s.sessions.Lookup(hashToken(c.Value), s.fingerprint, s.now())
	if !ok {
		return Session{}, "", false
	}
	return sess, c.Value, true
}

// IsSessionLive re-checks a request's session validity. SSE loops call this on
// each tick; when it turns false they emit an auth-failed event and close.
func (s *Service) IsSessionLive(r *http.Request) bool {
	_, _, ok := s.SessionFromRequest(r)
	return ok
}

func (s *Service) isPublic(r *http.Request) bool {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/":
		return true
	case r.Method == http.MethodPost && r.URL.Path == "/api/auth/login":
		return true
	case r.Method == http.MethodGet && r.URL.Path == "/api/auth/session":
		return true
	case r.Method == http.MethodOptions:
		return true
	}
	return false
}

func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func (s *Service) csrfOK(r *http.Request, sess Session) bool {
	got := r.Header.Get(CSRFHeader)
	return len(got) > 0 && subtleEqual(got, sess.CSRFToken)
}

// ---- Endpoints ----

// HandleLogin verifies the key and issues a session. It enforces same-origin
// JSON (so a cross-site form cannot log a victim in or out) and per-IP failure
// limiting.
func (s *Service) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAuthJSON(w, http.StatusMethodNotAllowed, 1, "method not allowed", nil)
		return
	}
	if !sameOrigin(r) {
		writeAuthJSON(w, http.StatusForbidden, 1, "cross-origin login not allowed", nil)
		return
	}
	var p struct {
		Key      string `json:"key"`
		Remember bool   `json:"remember"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeAuthJSON(w, http.StatusBadRequest, 1, "invalid json", nil)
		return
	}
	ip := s.clientIP(r)

	if VerifyKey(strings.TrimSpace(p.Key), s.key) {
		s.limiter.reset(ip)
		token, csrf, expires, err := s.sessions.Create(s.fingerprint, p.Remember, s.now())
		if err != nil {
			writeAuthJSON(w, http.StatusInternalServerError, 1, "session create failed", nil)
			return
		}
		setSessionCookie(w, token, p.Remember, expires, s.secureCookies)
		writeAuthJSON(w, http.StatusOK, 0, "ok", map[string]interface{}{
			"expires_at": expires.Unix(),
			"csrf_token": csrf,
			"remember":   p.Remember,
		})
		return
	}

	if s.limiter.recordFailure(ip) {
		writeAuthJSON(w, http.StatusTooManyRequests, 1, "too many login attempts, try again later", nil)
		return
	}
	writeAuthJSON(w, http.StatusUnauthorized, 1, "invalid access key", nil)
}

// HandleSession reports login status without renewing. An anonymous request
// gets {authenticated:false}; an authenticated one gets the expiry and the CSRF
// token so the page can arm its write requests after a reload.
func (s *Service) HandleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAuthJSON(w, http.StatusMethodNotAllowed, 1, "method not allowed", nil)
		return
	}
	sess, _, ok := s.SessionFromRequest(r)
	if !ok {
		writeAuthJSON(w, http.StatusOK, 0, "anonymous", map[string]interface{}{"authenticated": false})
		return
	}
	writeAuthJSON(w, http.StatusOK, 0, "ok", map[string]interface{}{
		"authenticated": true,
		"expires_at":    sess.ExpiresAt.Unix(),
		"remember":      sess.Remember,
		"csrf_token":    sess.CSRFToken,
	})
}

// HandleTouch renews the session on real foreground activity. It requires a
// valid session and CSRF (enforced by the middleware), updates the active time
// and idle expiry, and refreshes the cookie so remembered browsers stay signed
// in across continued use.
func (s *Service) HandleTouch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAuthJSON(w, http.StatusMethodNotAllowed, 1, "method not allowed", nil)
		return
	}
	sess, token, ok := s.SessionFromRequest(r)
	if !ok {
		writeAuthJSON(w, http.StatusUnauthorized, 1, "unauthorized", nil)
		return
	}
	expires, err := s.sessions.Touch(hashToken(token), s.now())
	if err != nil {
		writeAuthJSON(w, http.StatusUnauthorized, 1, "session expired", nil)
		return
	}
	setSessionCookie(w, token, sess.Remember, expires, s.secureCookies)
	writeAuthJSON(w, http.StatusOK, 0, "ok", map[string]interface{}{"expires_at": expires.Unix()})
}

// HandleLogout revokes the current browser's session and clears the cookie.
func (s *Service) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAuthJSON(w, http.StatusMethodNotAllowed, 1, "method not allowed", nil)
		return
	}
	_, token, ok := s.SessionFromRequest(r)
	if ok {
		_ = s.sessions.Delete(hashToken(token))
	}
	clearSessionCookie(w, s.secureCookies)
	writeAuthJSON(w, http.StatusOK, 0, "logged out", nil)
}

// ---- helpers ----

func (s *Service) clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if s.isTrustedProxy(host) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	return host
}

func (s *Service) isTrustedProxy(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range s.trustedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// sameOrigin reports whether the request's Origin/Referer matches the request
// Host. Login requires this so a third-party page cannot drive a JSON login.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return hostsEqual(u.Host, r.Host)
}

func hostsEqual(a, b string) bool {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if a == b {
		return true
	}
	// Tolerate a default-port mismatch (example.com vs example.com:443).
	_, porta, _ := net.SplitHostPort(a)
	_, portb, _ := net.SplitHostPort(b)
	if porta == "" {
		a = appendDefaultPort(a, "443")
	}
	if portb == "" {
		b = appendDefaultPort(b, "443")
	}
	return a == b
}

func appendDefaultPort(hostport, port string) string {
	if strings.Contains(hostport, ":") {
		return hostport
	}
	return hostport + ":" + port
}

func setSessionCookie(w http.ResponseWriter, token string, remember bool, expires time.Time, secure bool) {
	c := &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
	}
	if remember {
		c.Expires = expires
		c.MaxAge = int(time.Until(expires).Seconds())
	}
	http.SetCookie(w, c)
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	c := &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	}
	http.SetCookie(w, c)
}

func writeAuthJSON(w http.ResponseWriter, status, code int, msg string, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code": code,
		"msg":  msg,
		"data": data,
	})
}

func buildTrustedNets(extra []string) ([]*net.IPNet, error) {
	nets := []*net.IPNet{
		mustParseCIDR("127.0.0.0/8"),
		mustParseCIDR("::1/128"),
	}
	for _, item := range extra {
		s := strings.TrimSpace(item)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			if ip := net.ParseIP(s); ip != nil {
				if v4 := ip.To4(); v4 != nil {
					s = v4.String() + "/32"
				} else {
					s = ip.String() + "/128"
				}
			} else {
				return nil, errInvalidProxy(s)
			}
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, errInvalidProxy(s)
		}
		nets = append(nets, n)
	}
	return nets, nil
}

func mustParseCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(s)
	}
	return n
}

func subtleEqual(a, b string) bool {
	// CSRF tokens are random and fixed-length per session, so the length is
	// not a secret; a constant-time compare of the byte slices is sufficient.
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
