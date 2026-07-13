//go:build unix

package app

import (
	"os"
	"strconv"
)

// processImagePath returns the full path of the executable backing pid.
//
// It is best-effort: /proc/<pid>/exe exists on Linux but not on every Unix
// (notably macOS), so a false second return means "unknown" and callers must
// fall back to a liveness-only check rather than assuming the pid is stale.
// See the Windows counterpart for why this guards against PID reuse.
func processImagePath(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}
	target, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return "", false
	}
	return target, true
}
