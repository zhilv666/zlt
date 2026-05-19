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
	"sync"
	"syscall"
	"time"

	"tray/internal/task"
)

const (
	StatusStopped  = "stopped"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusStopping = "stopping"
	StatusExited   = "exited"
	StatusFailed   = "failed"
)

type RuntimeState struct {
	TaskID    string     `json:"task_id"`
	Status    string     `json:"status"`
	PID       int        `json:"pid"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	ExitedAt  *time.Time `json:"exited_at,omitempty"`
	ExitCode  *int       `json:"exit_code,omitempty"`
	LastError string     `json:"last_error,omitempty"`
}

type managedProcess struct {
	task   task.Config
	cmd    *exec.Cmd
	stdout *os.File
	stderr *os.File
	done   chan struct{}
	state  RuntimeState
}

type Manager struct {
	mu    sync.RWMutex
	procs map[string]*managedProcess
}

func NewManager(tasks []task.Config) *Manager {
	procs := make(map[string]*managedProcess, len(tasks))
	for _, cfg := range tasks {
		state := RuntimeState{
			TaskID: cfg.ID,
			Status: StatusStopped,
		}
		if pid, ok := findExistingProcess(cfg); ok {
			state.Status = StatusRunning
			state.PID = pid
		}

		procs[cfg.ID] = &managedProcess{
			task: cfg,
			state: state,
		}
	}
	return &Manager{procs: procs}
}

func (m *Manager) Tasks() []task.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]task.Config, 0, len(m.procs))
	for _, proc := range m.procs {
		out = append(out, proc.task)
	}
	return out
}

func (m *Manager) States() []RuntimeState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]RuntimeState, 0, len(m.procs))
	for _, proc := range m.procs {
		out = append(out, proc.state)
	}
	return out
}

func (m *Manager) State(taskID string) (RuntimeState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	proc, ok := m.procs[taskID]
	if !ok {
		return RuntimeState{}, false
	}
	return proc.state, true
}

func (m *Manager) ClearLogs(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	proc, ok := m.procs[taskID]
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}

	if err := os.MkdirAll(filepath.Join("data", "logs", taskID), 0o755); err != nil {
		return err
	}

	stdoutPath := filepath.Join("data", "logs", taskID, "stdout.log")
	stderrPath := filepath.Join("data", "logs", taskID, "stderr.log")

	if proc.stdout != nil {
		if err := proc.stdout.Truncate(0); err != nil {
			return err
		}
		if _, err := proc.stdout.Seek(0, 0); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(stdoutPath, []byte{}, 0o644); err != nil {
			return err
		}
	}

	if proc.stderr != nil {
		if err := proc.stderr.Truncate(0); err != nil {
			return err
		}
		if _, err := proc.stderr.Seek(0, 0); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(stderrPath, []byte{}, 0o644); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) UpsertTask(cfg task.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()

	proc, ok := m.procs[cfg.ID]
	if !ok {
		m.procs[cfg.ID] = &managedProcess{
			task: cfg,
			state: RuntimeState{
				TaskID: cfg.ID,
				Status: StatusStopped,
			},
		}
		return
	}

	proc.task = cfg
}

func (m *Manager) RemoveTask(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	proc, ok := m.procs[taskID]
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}
	if proc.cmd != nil || isRunningStatus(proc.state.Status) {
		return errors.New("task is running, stop it before deleting")
	}

	delete(m.procs, taskID)
	return nil
}

func (m *Manager) Start(taskID string) error {
	m.mu.Lock()
	proc, ok := m.procs[taskID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("task %q not found", taskID)
	}
	if proc.state.Status == StatusRunning || proc.state.Status == StatusStarting {
		m.mu.Unlock()
		return errors.New("task already running")
	}

	if pid, ok := findExistingProcess(proc.task); ok {
		proc.state.Status = StatusRunning
		proc.state.PID = pid
		proc.state.LastError = ""
		m.mu.Unlock()
		return nil
	}

	proc.state.Status = StatusStarting
	proc.state.LastError = ""
	proc.state.ExitCode = nil
	proc.state.ExitedAt = nil

	workdir := proc.task.WorkDir
	if workdir == "" {
		workdir = "."
	}
	if err := os.MkdirAll(filepath.Join("data", "logs", proc.task.ID), 0o755); err != nil {
		proc.state.Status = StatusFailed
		proc.state.LastError = err.Error()
		m.mu.Unlock()
		return err
	}

	stdoutPath := filepath.Join("data", "logs", proc.task.ID, "stdout.log")
	stderrPath := filepath.Join("data", "logs", proc.task.ID, "stderr.log")

	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		proc.state.Status = StatusFailed
		proc.state.LastError = err.Error()
		m.mu.Unlock()
		return err
	}

	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		_ = stdoutFile.Close()
		proc.state.Status = StatusFailed
		proc.state.LastError = err.Error()
		m.mu.Unlock()
		return err
	}

	resolvedProgram := resolveProgramPath(proc.task.Program, workdir)
	cmd := exec.Command(resolvedProgram, proc.task.Args...)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), proc.task.Env...)
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	if err := cmd.Start(); err != nil {
		_ = stdoutFile.Close()
		_ = stderrFile.Close()
		proc.state.Status = StatusFailed
		proc.state.LastError = fmt.Sprintf("%s (program=%s)", err.Error(), resolvedProgram)
		m.mu.Unlock()
		return err
	}

	now := time.Now()
	proc.cmd = cmd
	proc.stdout = stdoutFile
	proc.stderr = stderrFile
	proc.done = make(chan struct{})
	proc.state.Status = StatusRunning
	proc.state.PID = cmd.Process.Pid
	proc.state.StartedAt = &now
	proc.state.LastError = ""
	m.mu.Unlock()

	go m.wait(taskID, cmd, stdoutFile, stderrFile, proc.done)
	return nil
}

func (m *Manager) Stop(taskID string) error {
	m.mu.Lock()
	proc, ok := m.procs[taskID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("task %q not found", taskID)
	}
	if proc.state.Status == StatusStopped || proc.state.Status == StatusExited || proc.state.Status == StatusFailed {
		m.mu.Unlock()
		return nil
	}
	if proc.cmd == nil && proc.state.PID > 0 {
		pid := proc.state.PID
		proc.state.Status = StatusStopping
		m.mu.Unlock()

		if err := killProcessTree(pid); err != nil {
			return err
		}

		m.mu.Lock()
		if current, exists := m.procs[taskID]; exists {
			now := time.Now()
			current.state.Status = StatusStopped
			current.state.PID = 0
			current.state.ExitedAt = &now
			current.state.LastError = ""
		}
		m.mu.Unlock()
		return nil
	}
	if proc.cmd == nil || proc.cmd.Process == nil {
		proc.state.Status = StatusStopped
		proc.state.PID = 0
		m.mu.Unlock()
		return nil
	}

	proc.state.Status = StatusStopping
	cmd := proc.cmd
	done := proc.done
	timeout := time.Duration(proc.task.StopTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	m.mu.Unlock()

	_ = cmd.Process.Signal(os.Interrupt)

	select {
	case <-time.After(timeout):
		if err := killProcessTree(cmd.Process.Pid); err != nil {
			return err
		}
	case <-done:
	}

	return nil
}

func (m *Manager) wait(taskID string, cmd *exec.Cmd, stdoutFile, stderrFile *os.File, done chan struct{}) {
	err := cmd.Wait()
	close(done)

	_ = stdoutFile.Close()
	_ = stderrFile.Close()

	m.mu.Lock()
	defer m.mu.Unlock()

	proc, ok := m.procs[taskID]
	if !ok || proc.cmd != cmd {
		return
	}

	now := time.Now()
	proc.stdout = nil
	proc.stderr = nil
	proc.cmd = nil
	proc.done = nil
	proc.state.PID = 0
	proc.state.ExitedAt = &now

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	proc.state.ExitCode = &exitCode

	if err != nil {
		if proc.state.Status == StatusStopping {
			proc.state.Status = StatusStopped
			proc.state.LastError = ""
			return
		}
		proc.state.Status = StatusExited
		proc.state.LastError = err.Error()
	} else {
		proc.state.Status = StatusStopped
		proc.state.LastError = ""
	}
}

func resolveProgramPath(program string, workdir string) string {
	if program == "" {
		return program
	}
	if filepath.IsAbs(program) {
		return program
	}
	if strings.ContainsAny(program, `/\`) {
		return filepath.Clean(filepath.Join(workdir, program))
	}

	candidate := filepath.Join(workdir, program)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return program
}

func isRunningStatus(status string) bool {
	return status == StatusRunning || status == StatusStarting || status == StatusStopping
}

func killProcessTree(pid int) error {
	if pid <= 0 {
		return errors.New("invalid pid")
	}

	if runtime.GOOS == "windows" {
		killCmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/T", "/F")
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

	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func findExistingProcess(cfg task.Config) (int, bool) {
	if runtime.GOOS != "windows" {
		return 0, false
	}

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

	out, err := exec.Command("powershell.exe", "-NoProfile", "-Command", script).CombinedOutput()
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
