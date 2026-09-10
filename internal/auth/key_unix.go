//go:build unix

package auth

import "os"

// secureKeyFile enforces 0600 on Unix. WriteFile sets the mode at creation,
// but an existing file keeps its prior mode, so chmod explicitly to be safe.
func secureKeyFile(path string) error {
	return os.Chmod(path, 0o600)
}
