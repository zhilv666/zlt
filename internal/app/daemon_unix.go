//go:build unix

package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func startDetached(pidFile string, addr string) error {
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

	args := []string{"run"}
	if addr != "" {
		args = append(args, "--addr", addr)
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return err
	}

	lock, err := writePIDFile(pidFile, pidFilePayload{
		PID:  cmd.Process.Pid,
		Addr: addr,
	})
	if err != nil {
		_ = killDetachedPID(cmd.Process.Pid)
		return err
	}

	_ = lock
	return nil
}

func stopDetached(pidFile string) error {
	lock, err := readPIDFile(pidFile)
	if err != nil {
		return err
	}

	if err := syscall.Kill(lock.PID, syscall.SIGTERM); err != nil {
		if err == syscall.ESRCH {
			_ = os.Remove(pidFile)
			return os.ErrNotExist
		}
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

func killDetachedPID(pid int) error {
	if pid <= 0 {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGKILL)
}
