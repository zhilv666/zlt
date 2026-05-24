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

	"zhulingtai/internal/task"
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
	resolvedWorkdir := workdir
	if abs, err := filepath.Abs(workdir); err == nil {
		resolvedWorkdir = abs
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
		if !commandMatchesTask(commandLine, resolvedProgram, cfg.Args) {
			continue
		}
		if !processMatchesWorkingDir(pid, resolvedWorkdir) {
			continue
		}
		return pid, true
	}

	return 0, false
}

func processMatchesWorkingDir(pid int, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	actual, ok := readLinuxProcessWorkingDir(pid)
	if !ok {
		return false
	}
	return sameWorkingDir(actual, expected)
}

func readLinuxProcessWorkingDir(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}

	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return "", false
	}
	if cwd == "" {
		return "", false
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	return cwd, true
}
