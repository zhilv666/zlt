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

// keySize is the entropy length of a generated access key, in bytes.
const keySize = 32

// LoadOrCreateKey loads the access key from path, or generates a fresh
// 32-byte key on first run and persists it with restrictive permissions.
//
// The key file holds the base64url string the operator enters at the login
// prompt (and what `zlt auth show` prints). An existing but empty, malformed
// or wrong-length file is a hard error: the key is never silently replaced,
// because doing so would invalidate every session and leave the operator
// without the value they need to log in.
func LoadOrCreateKey(path string) ([]byte, error) {
	if raw, err := os.ReadFile(path); err == nil {
		return parseStoredKey(raw, path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	buf := make([]byte, keySize)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate access key: %w", err)
	}
	encoded := DisplayKey(buf)
	if err := writeKeyFile(path, []byte(encoded)); err != nil {
		return nil, err
	}
	return buf, nil
}

func parseStoredKey(raw []byte, path string) ([]byte, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, fmt.Errorf("auth key file %s is empty", path)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("auth key file %s is not valid base64url: %w", path, err)
	}
	if len(decoded) != keySize {
		return nil, fmt.Errorf("auth key file %s has wrong length: got %d bytes, want %d", path, len(decoded), keySize)
	}
	return decoded, nil
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

// DisplayKey returns the base64url string the operator enters at login.
func DisplayKey(key []byte) string {
	return base64.RawURLEncoding.EncodeToString(key)
}

// KeyFingerprint is a stable hash of the key, stored per session so that
// rotating the key (auth reset) invalidates every session issued under the old
// key without having to enumerate the session table.
func KeyFingerprint(key []byte) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])
}

// VerifyKey compares a user-supplied key string against the stored key in
// constant time. Accepts the base64url string the user pastes.
func VerifyKey(supplied string, key []byte) bool {
	want := DisplayKey(key)
	return subtle.ConstantTimeCompare([]byte(supplied), []byte(want)) == 1
}

// ReadKeyFile returns the display string stored in the key file, for `zlt auth
// show` and the tray "view key" entry. The raw bytes are not handed out here;
// callers that need them should hold the in-memory key.
func ReadKeyFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", fmt.Errorf("auth key file %s is empty", path)
	}
	if _, err := base64.RawURLEncoding.DecodeString(text); err != nil {
		return "", fmt.Errorf("auth key file %s is not valid base64url: %w", path, err)
	}
	return text, nil
}
