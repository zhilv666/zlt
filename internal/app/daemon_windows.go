//go:build windows

package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const createNoWindow = 0x08000000

func startDetached(pidFile string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	if err := os.MkdirAll("data", 0o755); err != nil {
		return err
	}

	stdoutPath := filepath.Join("data", "tray-service.out.log")
	stderrPath := filepath.Join("data", "tray-service.err.log")
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

	cmd := exec.Command(exe, "run")
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

	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0o644); err != nil {
		_ = cmd.Process.Kill()
		return err
	}

	return nil
}

func stopDetached(pidFile string) error {
	lock, err := readPIDFile(pidFile)
	if err != nil {
		return err
	}

	proc, err := os.FindProcess(lock.PID)
	if err != nil {
		return err
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		_ = os.Remove(pidFile)
		return os.ErrNotExist
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
