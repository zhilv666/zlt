//go:build windows

package process

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"tray/internal/task"
)

const createNoWindow = 0x08000000

func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createNoWindow,
		HideWindow:    true,
	}
}

func requestProcessStop(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(os.Interrupt)
}

func killProcessTree(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid")
	}

	killCmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/T", "/F")
	preparePlatformCommand(killCmd)
	output, err := killCmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg != "" {
			return fmt.Errorf("taskkill failed: %s", msg)
		}
		return err
	}
	return nil
}

func findExistingProcess(cfg task.Config) (int, bool) {
	workdir := cfg.WorkDir
	if workdir == "" {
		workdir = "."
	}
	resolvedProgram := resolveProgramPath(cfg.Program, workdir)
	if !filepath.IsAbs(resolvedProgram) {
		if abs, err := filepath.Abs(resolvedProgram); err == nil {
			resolvedProgram = abs
		}
	}

	script := "$exe = [System.IO.Path]::GetFullPath('" + escapePowerShellSingleQuoted(resolvedProgram) + "'); " +
		"$proc = Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -and ([System.StringComparer]::OrdinalIgnoreCase.Equals($_.ExecutablePath, $exe)) } | Select-Object -First 1 -ExpandProperty ProcessId; " +
		"if ($proc) { Write-Output $proc }"

	checkCmd := exec.Command("powershell.exe", "-NoProfile", "-Command", script)
	preparePlatformCommand(checkCmd)
	out, err := checkCmd.CombinedOutput()
	if err != nil {
		return 0, false
	}

	text := strings.TrimSpace(string(out))
	if text == "" {
		return 0, false
	}

	pid, err := strconv.Atoi(text)
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func escapePowerShellSingleQuoted(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func preparePlatformCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
		HideWindow:    true,
	}
}
