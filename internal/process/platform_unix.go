//go:build unix

package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

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

	// An adopted process may share its parent's group. Never signal that group;
	// stop only its descendants and the target itself in that case.
	pgid, err := syscall.Getpgid(pid)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if err != nil {
		return err
	}
	if pgid == pid {
		err = syscall.Kill(-pid, syscall.SIGKILL)
	} else {
		for _, child := range descendantPIDs(pid) {
			if err := syscall.Kill(child, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				return err
			}
		}
		err = syscall.Kill(pid, syscall.SIGKILL)
	}
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for processIsAlive(pid) {
		if time.Now().After(deadline) {
			return fmt.Errorf("process %d did not exit after SIGKILL", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

func descendantPIDs(pid int) []int {
	out, err := exec.Command("ps", "-ax", "-o", "pid=", "-o", "ppid=").Output()
	if err != nil {
		return nil
	}
	children := make(map[int][]int)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		child, e1 := strconv.Atoi(fields[0])
		parent, e2 := strconv.Atoi(fields[1])
		if e1 == nil && e2 == nil {
			children[parent] = append(children[parent], child)
		}
	}
	var result []int
	var visit func(int)
	visit = func(parent int) {
		for _, child := range children[parent] {
			visit(child)
			result = append(result, child)
		}
	}
	visit(pid)
	return result
}

func processIsAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if errors.Is(err, os.ErrNotExist) {
			return false
		}
		if err == nil {
			end := strings.LastIndexByte(string(data), ')')
			if end >= 0 && len(data) > end+2 && (data[end+2] == 'Z' || data[end+2] == 'X') {
				return false
			}
		}
	}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "stat=").Output()
		if err == nil && strings.HasPrefix(strings.TrimSpace(string(out)), "Z") {
			return false
		}
	}
	return true
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

	// ps only enumerates PIDs. Identity and argv must come from platform APIs,
	// since a formatted command line cannot preserve empty or spaced arguments.
	out, err := exec.Command("ps", "-ax", "-o", "pid=").Output()
	if err != nil {
		return 0, false
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		pid, err := strconv.Atoi(line)
		if err != nil || pid <= 0 {
			continue
		}

		exe, argv, ok := readUnixProcessCommand(pid)
		if !ok || !sameProcessFile(exe, resolvedProgram) || len(argv) != len(cfg.Args)+1 {
			continue
		}
		matches := true
		for i, arg := range cfg.Args {
			if argv[i+1] != arg {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		if !processMatchesWorkingDir(pid, resolvedWorkdir) {
			continue
		}
		return pid, true
	}

	return 0, false
}

func sameProcessFile(left, right string) bool {
	if sameExecutable(left, right) {
		return true
	}
	l, err := os.Stat(left)
	if err != nil {
		return false
	}
	r, err := os.Stat(right)
	return err == nil && os.SameFile(l, r)
}

func processMatchesWorkingDir(pid int, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	return unixProcessMatchesWorkingDir(pid, expected)
}
