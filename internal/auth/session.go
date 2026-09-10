package auth

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// SessionMaxIdle is the server-side idle lifetime. A session that sees no
// activity for this long is expired and cannot be revived by a touch.
const SessionMaxIdle = 7 * 24 * time.Hour

// Session holds the persisted fields of a logged-in browser. The raw session
// token never lives here: the database stores only its hash.
type Session struct {
	TokenHash      string
	KeyFingerprint string
	Remember       bool
	LastActiveAt   time.Time
	ExpiresAt      time.Time
	CSRFToken      string
}

// SessionStore persists sessions in a dedicated SQLite database. Keeping it
// separate from the task database means `zlt auth reset` can drop the whole
// file without touching task or schedule data.
type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(dbPath string) (*SessionStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	s := &SessionStore{db: db}
	if err := s.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SessionStore) init() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			key_fingerprint TEXT NOT NULL,
			remember INTEGER NOT NULL,
			last_active_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			csrf_token TEXT NOT NULL
		)
	`); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`)
	return err
}

func (s *SessionStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Create issues a new session for the given key fingerprint. It returns the
// raw token (to place in the cookie) and the CSRF token (to hand the page).
func (s *SessionStore) Create(keyFingerprint string, remember bool, now time.Time) (token, csrf string, expiresAt time.Time, err error) {
	token, err = randomToken()
	if err != nil {
		return
	}
	csrf, err = randomToken()
	if err != nil {
		return
	}
	expiresAt = now.Add(SessionMaxIdle)
	_, err = s.db.Exec(`
		INSERT INTO sessions (token_hash, key_fingerprint, remember, last_active_at, expires_at, csrf_token)
		VALUES (?, ?, ?, ?, ?, ?)
	`, hashToken(token), keyFingerprint, boolToInt(remember), now.Unix(), expiresAt.Unix(), csrf)
	return
}

// Lookup finds a live session by token hash. A session is live only if its key
// fingerprint still matches the current key and it has not expired; expired or
// key-rotated sessions are treated as absent so callers uniformly redirect to
// login.
func (s *SessionStore) Lookup(tokenHash, currentKeyFingerprint string, now time.Time) (Session, bool) {
	var (
		sess       Session
		remember   int
		lastActive int64
		expires    int64
	)
	row := s.db.QueryRow(`
		SELECT token_hash, key_fingerprint, remember, last_active_at, expires_at, csrf_token
		FROM sessions WHERE token_hash = ?
	`, tokenHash)
	if err := row.Scan(&sess.TokenHash, &sess.KeyFingerprint, &remember, &lastActive, &expires, &sess.CSRFToken); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, false
		}
		return Session{}, false
	}
	sess.Remember = remember == 1
	sess.LastActiveAt = time.Unix(lastActive, 0)
	sess.ExpiresAt = time.Unix(expires, 0)
	if sess.KeyFingerprint != currentKeyFingerprint {
		return Session{}, false
	}
	if !now.Before(sess.ExpiresAt) {
		return Session{}, false
	}
	return sess, true
}

// Touch updates the active time and recomputes the idle expiry for a session.
// It returns the new expiry so the caller can refresh the browser cookie.
func (s *SessionStore) Touch(tokenHash string, now time.Time) (time.Time, error) {
	expiresAt := now.Add(SessionMaxIdle)
	res, err := s.db.Exec(`
		UPDATE sessions SET last_active_at = ?, expires_at = ? WHERE token_hash = ?
	`, now.Unix(), expiresAt.Unix(), tokenHash)
	if err != nil {
		return time.Time{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return time.Time{}, errors.New("session not found")
	}
	return expiresAt, nil
}

func (s *SessionStore) Delete(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// DeleteByKeyFingerprint removes every session issued under a given key
// fingerprint. Used by `zlt auth reset` to invalidate all browsers at once.
func (s *SessionStore) DeleteByKeyFingerprint(fp string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE key_fingerprint = ?`, fp)
	return err
}

// PurgeExpired removes sessions whose idle expiry has passed. Called at
// startup and periodically to keep the table bounded.
func (s *SessionStore) PurgeExpired(now time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now.Unix())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ValidateTokenHash is a thin wrapper for callers (e.g. SSE loops) that already
// hold the token hash and only want to know whether the session is still live.
func (s *SessionStore) ValidateTokenHash(tokenHash, currentKeyFingerprint string, now time.Time) bool {
	_, ok := s.Lookup(tokenHash, currentKeyFingerprint, now)
	return ok
}
