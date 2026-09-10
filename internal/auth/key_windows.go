//go:build windows

package auth

import (
	"fmt"
	"os/exec"
	"os/user"
	"strings"
)

// secureKeyFile restricts the key file on Windows to the current running
// account and SYSTEM only, by removing inherited ACEs and granting full
// control to those two principals. This solves the fact that Windows ignores
// POSIX permission bits, so a 0600 mode alone does not protect the file.
func secureKeyFile(path string) error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("resolve current user: %w", err)
	}
	account := strings.TrimSpace(u.Username)
	if account == "" {
		return fmt.Errorf("current user name is empty; cannot restrict key file")
	}
	cmd := exec.Command("icacls", path, "/inheritance:r", "/grant:r", account+":F", "/grant:r", "SYSTEM:F")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("set key file acl: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
