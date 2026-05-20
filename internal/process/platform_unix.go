//go:build unix

package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"tray/internal/task"
)

func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func requestProcessStop(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func killProcessTree(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid")
	}

	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		proc, findErr := os.FindProcess(pid)
		if findErr != nil {
			return findErr
		}
		return proc.Kill()
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

	out, err := exec.Command("ps", "-ax", "-o", "pid=", "-o", "command=").Output()
	if err != nil {
		return 0, false
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}

		commandLine := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
		if commandMatchesProgram(commandLine, resolvedProgram) {
			return pid, true
		}
	}

	return 0, false
}
