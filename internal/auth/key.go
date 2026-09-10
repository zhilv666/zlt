// Package auth implements key-based browser authentication for the zlt
// management interface: a locally generated access key, server-side sessions
// with idle expiry, CSRF protection and a unified middleware that guards every
// business route.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// randomBytes is the entropy length of a randomly generated access key, in bytes.
const randomBytes = 32

// minSecretLen is the minimum length of a manually chosen access key. Generated
// keys (43 chars) always pass; the bound only guards `auth set` passphrases.
const minSecretLen = 8

// LoadOrCreateKey loads the access key from path, or generates a fresh random
// key on first run and persists it with restrictive permissions.
//
// The file holds the exact text the operator enters at the login prompt (and
// what `zlt auth show` prints): either a random base64url string or a manually
// chosen passphrase. An existing but empty file is a hard error — the key is
// never silently replaced, because doing so would invalidate every session and
// leave the operator without the value they need to log in.
func LoadOrCreateKey(path string) (string, error) {
	if raw, err := os.ReadFile(path); err == nil {
		text := strings.TrimSpace(string(raw))
		if text == "" {
			return "", fmt.Errorf("auth key file %s is empty", path)
		}
		return text, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	secret, err := RandomSecret()
	if err != nil {
		return "", fmt.Errorf("generate access key: %w", err)
	}
	if err := WriteKeyFile(path, secret); err != nil {
		return "", err
	}
	return secret, nil
}

// RandomSecret returns a new random access key rendered as base64url. It stays
// base64url only as the display encoding of fresh random bytes; a key set
// manually via `auth set` is stored verbatim instead.
func RandomSecret() (string, error) {
	buf := make([]byte, randomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// WriteKeyFile stores a manually chosen access key verbatim (single line,
// no surrounding whitespace, at least minSecretLen characters) and restricts
// the file the same way a generated key is protected.
func WriteKeyFile(path, secret string) error {
	if strings.TrimSpace(secret) != secret {
		return errors.New("access key must not start or end with whitespace")
	}
	if strings.ContainsAny(secret, "\r\n") {
		return errors.New("access key must be a single line")
	}
	if len(secret) < minSecretLen {
		return fmt.Errorf("access key must be at least %d characters", minSecretLen)
	}
	return writeKeyFile(path, []byte(secret+"\n"))
}

func writeKeyFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return err
	}
	return secureKeyFile(path)
}

// Fingerprint is a stable hash of the access key, stored per session so that
// changing the key (auth set or reset) invalidates every session issued under
// the old key without having to enumerate the session table.
func Fingerprint(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// VerifyKey compares a user-supplied key string against the stored access key
// in constant time. Leading/trailing whitespace on the supplied value is
// ignored, matching what the web login form trims client-side.
func VerifyKey(supplied, secret string) bool {
	supplied = strings.TrimSpace(supplied)
	return subtle.ConstantTimeCompare([]byte(supplied), []byte(secret)) == 1
}

// ReadKeyFile returns the access key text stored in the file, for `zlt auth
// show` and the tray "view key" entry.
func ReadKeyFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", fmt.Errorf("auth key file %s is empty", path)
	}
	return text, nil
}
