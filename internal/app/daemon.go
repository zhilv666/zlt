package app

import (
	"fmt"
	"os"
	"time"
)

// waitForDetachedStart blocks until the freshly spawned detached child has taken
// ownership of pidFile — proving it won the single-instance lock and is serving —
// or until it exits (typically because another instance was already running).
//
// The child writes the pid file itself (via acquirePIDFile in RunWithOptions),
// so the parent must not pre-write it; this poll is how `zlt start` reports a
// real startup failure instead of returning success for a process that died.
func waitForDetachedStart(pidFile string, childPID int, logPath string) error {
	for i := 0; i < 60; i++ {
		if lock, err := readPIDFile(pidFile); err == nil && lock.PID == childPID {
			return nil
		}
		if !processExists(childPID) {
			return fmt.Errorf("驻令台 后台进程启动失败，请查看日志 %s", logPath)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("驻令台 后台进程在规定时间内未就绪，请查看日志 %s", logPath)
}

// ensureNotRunning fails fast when a live instance already holds pidFile, and
// clears the file when it turns out to be stale so the caller can proceed.
func ensureNotRunning(pidFile string) error {
	existing, err := readPIDFile(pidFile)
	if err != nil {
		return nil
	}
	if processMatches(existing.PID, existing.Exe) {
		return fmt.Errorf("%w (pid %d)", errAlreadyRunning, existing.PID)
	}
	_ = os.Remove(pidFile)
	return nil
}
