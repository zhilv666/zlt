//go:build windows

package app

import (
	"errors"

	"golang.org/x/sys/windows"
)

// stillActive is the Windows STILL_ACTIVE / STATUS_PENDING exit code that
// GetExitCodeProcess reports while a process is still running.
const stillActive = 259

// processExistsPlatform reports whether a process with the given pid is alive.
//
// The obvious approach (os.FindProcess + Signal(0)) does NOT work on Windows:
// os/exec only supports the Kill signal there, so Signal(0) always returns an
// error and the process is reported as dead — which silently disables the
// single-instance guard. We instead open the process and inspect its exit code.
func processExistsPlatform(pid int) bool {
	if pid <= 0 {
		return false
	}

	// PROCESS_QUERY_LIMITED_INFORMATION is the least-privileged access right that
	// still lets us read the exit code, and it is grantable across integrity
	// levels within the same session.
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// The process exists but is owned by a more privileged account we may not
		// open: treat it as alive (mirrors the EPERM handling on Unix) so we never
		// start a second instance on top of a running one.
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return true
		}
		return false
	}
	defer windows.CloseHandle(handle)

	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}
