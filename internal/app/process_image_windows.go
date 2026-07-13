//go:build windows

package app

import "golang.org/x/sys/windows"

// processImagePath returns the full path of the executable backing pid.
//
// It is used to defend the single-instance lock against PID reuse: after an
// unclean exit (or a reboot, when the pid file survives on disk) the recorded
// pid may have been recycled by an unrelated process, and we must not refuse to
// start because of that. A false second return means the image could not be
// determined and callers should fall back to a liveness-only check.
func processImagePath(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}

	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(handle)

	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return "", false
	}
	return windows.UTF16ToString(buf[:size]), true
}
