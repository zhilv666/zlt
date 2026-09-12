//go:build windows

package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const createNoWindow = 0x08000000

func startDetached(pidFile string, addr string) error {
	// Fail fast with a clear message instead of spawning a doomed child that
	// would lose the single-instance race.
	if err := ensureNotRunning(pidFile); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	if err := os.MkdirAll("data", 0o755); err != nil {
		return err
	}

	stdoutPath := filepath.Join("data", "zlt-service.out.log")
	stderrPath := filepath.Join("data", "zlt-service.err.log")
	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer stdoutFile.Close()

	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer stderrFile.Close()

	// The child owns the pid file (and thus the single-instance lock) via
	// --pid-file; the parent must not pre-write it or the child would see the
	// lock already taken and refuse to start.
	args := []string{"run", "--pid-file", pidFile}
	if addr != "" {
		args = append(args, "--addr", addr)
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
		HideWindow:    true,
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	return waitForDetachedStart(pidFile, cmd.Process.Pid, stderrPath)
}

func stopDetached(pidFile string) error {
	lock, err := readPIDFile(pidFile)
	if err != nil {
		return err
	}

	// A lock whose process is gone (or whose pid was recycled by an unrelated
	// program) is stale: clear it and treat the stop as already done.
	if !processMatches(lock.PID, lock.Exe) {
		_ = os.Remove(pidFile)
		return nil
	}

	// Windows cannot deliver os.Interrupt to another process — os/exec supports
	// only Kill there — so the detached daemon cannot be asked to shut down
	// gracefully. Terminating the whole tree mirrors how managed tasks are
	// force-stopped (internal/process) and also reaps the task children the
	// daemon would otherwise leave running.
	if err := taskkillProcessTree(lock.PID); err != nil {
		return err
	}

	for i := 0; i < 40; i++ {
		if !processExists(lock.PID) {
			_ = os.Remove(pidFile)
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}

	return fmt.Errorf("service pid %d did not stop in time", lock.PID)
}

// taskkillProcessTree force-terminates pid and its descendants via taskkill,
// the same tool internal/process uses to stop managed tasks on Windows.
func taskkillProcessTree(pid int) error {
	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
		HideWindow:    true,
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(output)); msg != "" {
			return fmt.Errorf("taskkill failed: %s", msg)
		}
		return err
	}
	return nil
}
