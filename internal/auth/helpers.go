package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// tokenBytes is the entropy of a session token and a CSRF token, in bytes.
const tokenBytes = 32

// randomToken returns a base64url-encoded random token string.
func randomToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashToken returns the SHA-256 hex digest of a token string. Only the hash is
// ever stored, so a database leak cannot be turned back into valid cookies.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
