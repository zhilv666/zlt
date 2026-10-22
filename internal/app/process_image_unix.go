//go:build unix

package app

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// processImagePath returns the full path of the executable backing pid.
//
// Linux exposes /proc/<pid>/exe; macOS exposes the image path through ps.
func processImagePath(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}
	target, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err == nil {
		return target, true
	}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return strings.TrimSpace(string(out)), true
		}
	}
	return "", false
}
