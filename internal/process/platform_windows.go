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

	"zhulingtai/internal/task"
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

	script := "$OutputEncoding = [Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " +
		"Get-CimInstance Win32_Process | ForEach-Object { " +
		"$pid = $_.ProcessId; " +
		"$exe = if ($_.ExecutablePath) { $_.ExecutablePath } else { '' }; " +
		"$cmd = if ($_.CommandLine) { $_.CommandLine } else { '' }; " +
		"Write-Output ($pid.ToString() + \"`t\" + ($exe -replace \"`t\", \" \") + \"`t\" + ($cmd -replace \"`t\", \" \")) " +
		"}"

	checkCmd := exec.Command("powershell.exe", "-NoProfile", "-Command", script)
	preparePlatformCommand(checkCmd)
	out, err := checkCmd.CombinedOutput()
	if err != nil {
		return 0, false
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}

		pid, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || pid <= 0 {
			continue
		}

		executablePath := strings.TrimSpace(parts[1])
		commandLine := strings.TrimSpace(parts[2])

		if executablePath != "" && !sameExecutable(executablePath, resolvedProgram) {
			continue
		}
		if !commandMatchesTask(commandLine, resolvedProgram, cfg.Args) {
			continue
		}
		return pid, true
	}

	return 0, false
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
