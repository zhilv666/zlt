package app

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

// waitForDetachedStart blocks until the freshly spawned detached child has both
// taken ownership of pidFile (proving it won the single-instance lock) AND is
// actually serving HTTP (proving auth init + port listen succeeded). Previously
// it only checked the pid file, so `zlt start` could report success for a child
// that died moments later because the port was in use or auth init failed.
//
// The HTTP probe hits "/" — the index page is public even behind auth, so a 200
// means the middleware is wired and the listener is accepting connections.
func waitForDetachedStart(pidFile string, childPID int, logPath string) error {
	// Phase 1: the child must own the pid file (single-instance lock won).
	var addr string
	for i := 0; i < 60; i++ {
		if lock, err := readPIDFile(pidFile); err == nil && lock.PID == childPID {
			addr = lock.Addr
			if addr == "" {
				addr = defaultHTTPAddr
			}
			break
		}
		if !processExists(childPID) {
			return fmt.Errorf("驻令台 后台进程启动失败，请查看日志 %s", logPath)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if addr == "" {
		return fmt.Errorf("驻令台 后台进程在规定时间内未获取单例锁，请查看日志 %s", logPath)
	}

	// Phase 2: HTTP must actually serve. The pid file proves the lock was
	// acquired, not that the listener bound or auth initialized.
	probeURL := "http://" + addr + "/"
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < 50; i++ {
		if !processExists(childPID) {
			return fmt.Errorf("驻令台 后台进程启动失败，请查看日志 %s", logPath)
		}
		resp, err := client.Get(probeURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
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
