package process

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"zhulingtai/internal/logging"
	"zhulingtai/internal/task"
)

const (
	StatusStopped  = "stopped"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusStopping = "stopping"
	StatusExited   = "exited"
	StatusFailed   = "failed"
)

const (
	maxLogSizeBytes = 10 * 1024 * 1024
	maxLogBackups   = 3
)

var ErrTaskNotFound = errors.New("task not found")

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
	task           task.Config
	cmd            *exec.Cmd
	logFile        *logging.RotatingWriter
	done           chan struct{}
	healthStop     chan struct{}
	state          RuntimeState
	restartCount   int
	healthFailures int
	stopReason     stopReason
	stopMessage    string
}

type Manager struct {
	mu    sync.RWMutex
	procs map[string]*managedProcess

	// Task-log rotation limits, adjustable at runtime via SetLogLimits. Read on
	// every Start so new/restarted tasks pick up changes; running tasks are
	// updated in place by SetLogLimits.
	logMaxSize    atomic.Int64
	logMaxBackups atomic.Int32
}

type stopReason string

const (
	stopReasonUser   stopReason = "user"
	stopReasonHealth stopReason = "health"
)

func NewManager(tasks []task.Config) *Manager {
	manager := &Manager{
		procs: make(map[string]*managedProcess, len(tasks)),
	}
	manager.logMaxSize.Store(maxLogSizeBytes)
	manager.logMaxBackups.Store(maxLogBackups)
	for _, cfg := range tasks {
		state := RuntimeState{
			TaskID: cfg.ID,
			Status: StatusStopped,
		}
		if pid, ok := findExistingProcess(cfg); ok {
			state.Status = StatusRunning
			state.PID = pid
		}

		manager.procs[cfg.ID] = &managedProcess{
			task:  cfg,
			state: state,
		}
	}
	for _, proc := range manager.procs {
		if proc.state.Status == StatusRunning {
			manager.startHealthMonitorLocked(proc)
		}
	}
	return manager
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

// SetLogLimits updates task-log rotation limits. New/restarted tasks read the
// new values, and any currently running task's writer is updated in place so the
// change is felt immediately.
func (m *Manager) SetLogLimits(maxSize int64, maxBackups int) {
	m.logMaxSize.Store(maxSize)
	m.logMaxBackups.Store(int32(maxBackups))

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, proc := range m.procs {
		if proc.logFile != nil {
			proc.logFile.SetLimits(maxSize, maxBackups)
		}
	}
}

func (m *Manager) ClearLogs(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	proc, ok := m.procs[taskID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}

	if err := os.MkdirAll(filepath.Join("data", "logs", taskID), 0o755); err != nil {
		return err
	}

	logPath := filepath.Join("data", "logs", taskID, "app.log")
	legacyStdoutPath := filepath.Join("data", "logs", taskID, "stdout.log")
	legacyStderrPath := filepath.Join("data", "logs", taskID, "stderr.log")

	// Clear everything for this task: the current run plus all archived runs. The
	// "start clean each run" need is already met by archiving on start, so this
	// button's job is a full reset (and reclaiming disk).
	if proc.logFile != nil {
		if err := proc.logFile.Purge(); err != nil {
			return err
		}
	} else if err := logging.PurgeLog(logPath); err != nil {
		return err
	}

	_ = os.Remove(legacyStdoutPath)
	_ = os.Remove(legacyStderrPath)

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
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	if proc.cmd != nil || isRunningStatus(proc.state.Status) {
		return errors.New("task is running, stop it before deleting")
	}

	delete(m.procs, taskID)
	return nil
}

func (m *Manager) Start(taskID string) error {
	return m.start(taskID, true)
}

// start launches a task. freshRun archives the previous run's log so the viewer
// opens clean; it is true for every deliberate start (manual, restart, schedule,
// autostart) and false only for a crash-triggered auto-restart, which keeps
// appending so the crash and its retry stay in one place.
func (m *Manager) start(taskID string, freshRun bool) error {
	m.mu.Lock()
	proc, ok := m.procs[taskID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	if proc.state.Status == StatusRunning || proc.state.Status == StatusStarting {
		m.mu.Unlock()
		return errors.New("task already running")
	}

	if pid, ok := findExistingProcess(proc.task); ok {
		proc.state.Status = StatusRunning
		proc.state.PID = pid
		proc.state.LastError = ""
		proc.state.ExitCode = nil
		proc.state.ExitedAt = nil
		proc.stopReason = ""
		proc.stopMessage = ""
		m.startHealthMonitorLocked(proc)
		m.mu.Unlock()
		return nil
	}

	proc.state.Status = StatusStarting
	proc.state.LastError = ""
	proc.state.ExitCode = nil
	proc.state.ExitedAt = nil
	proc.stopReason = ""
	proc.stopMessage = ""

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

	logPath := filepath.Join("data", "logs", proc.task.ID, "app.log")

	// The rotating writer enforces the size cap on every write, so a long-lived
	// chatty task can never grow its log without bound. The previous size check
	// only ran here at start time and was skipped for the entire run.
	logFile, err := logging.NewRotatingWriter(logPath, m.logMaxSize.Load(), int(m.logMaxBackups.Load()))
	if err != nil {
		proc.state.Status = StatusFailed
		proc.state.LastError = err.Error()
		m.mu.Unlock()
		return err
	}

	// A fresh run archives the previous run's output (current app.log → app.log.1,
	// bounded by the configured backup count) so the viewer starts clean. A crash
	// auto-restart skips this and appends instead. Either way we stamp a banner so
	// run boundaries are visible even inside the archived history.
	if freshRun {
		if err := logFile.Rotate(); err != nil {
			_ = logFile.Close()
			proc.state.Status = StatusFailed
			proc.state.LastError = err.Error()
			m.mu.Unlock()
			return err
		}
	}
	fmt.Fprintf(logFile, "==== %s %s ====\n", runBannerLabel(freshRun), time.Now().Format("2006-01-02 15:04:05"))

	resolvedProgram := resolveProgramPath(proc.task.Program, workdir)
	cmd := exec.Command(resolvedProgram, proc.task.Args...)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), proc.task.Env...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	prepareCommand(cmd)

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		proc.state.Status = StatusFailed
		proc.state.LastError = fmt.Sprintf("%s (program=%s)", err.Error(), resolvedProgram)
		m.mu.Unlock()
		return err
	}

	now := time.Now()
	proc.cmd = cmd
	proc.logFile = logFile
	proc.done = make(chan struct{})
	proc.state.Status = StatusRunning
	proc.state.PID = cmd.Process.Pid
	proc.state.StartedAt = &now
	proc.state.LastError = ""
	m.startHealthMonitorLocked(proc)
	m.mu.Unlock()

	go m.wait(taskID, cmd, logFile, proc.done)
	return nil
}

func (m *Manager) Stop(taskID string) error {
	return m.stopWithReason(taskID, stopReasonUser, "", nil)
}

func (m *Manager) stopWithReason(taskID string, reason stopReason, message string, healthStop chan struct{}) error {
	m.mu.Lock()
	proc, ok := m.procs[taskID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	if proc.state.Status == StatusStopped || proc.state.Status == StatusExited || proc.state.Status == StatusFailed {
		m.mu.Unlock()
		return nil
	}
	if healthStop != nil && proc.healthStop != healthStop {
		m.mu.Unlock()
		return nil
	}
	if proc.cmd == nil && proc.state.PID > 0 {
		pid := proc.state.PID
		proc.state.Status = StatusStopping
		proc.stopReason = reason
		proc.stopMessage = message
		m.stopHealthMonitorLocked(proc)
		m.mu.Unlock()

		if err := killProcessTree(pid); err != nil {
			return err
		}

		m.mu.Lock()
		if current, exists := m.procs[taskID]; exists {
			now := time.Now()
			current.state.PID = 0
			current.state.ExitedAt = &now
			_, _, _ = m.finalizeStopLocked(current)
		}
		m.mu.Unlock()
		return nil
	}
	if proc.cmd == nil || proc.cmd.Process == nil {
		proc.state.Status = StatusStopping
		proc.stopReason = reason
		proc.stopMessage = message
		m.stopHealthMonitorLocked(proc)
		proc.state.PID = 0
		_, _, _ = m.finalizeStopLocked(proc)
		m.mu.Unlock()
		return nil
	}

	proc.state.Status = StatusStopping
	proc.stopReason = reason
	proc.stopMessage = message
	m.stopHealthMonitorLocked(proc)
	cmd := proc.cmd
	done := proc.done
	timeout := time.Duration(proc.task.StopTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	m.mu.Unlock()

	_ = requestProcessStop(cmd)

	select {
	case <-time.After(timeout):
		if err := killProcessTree(cmd.Process.Pid); err != nil {
			return err
		}
	case <-done:
	}

	return nil
}

func (m *Manager) wait(taskID string, cmd *exec.Cmd, logFile *logging.RotatingWriter, done chan struct{}) {
	err := cmd.Wait()
	close(done)

	_ = logFile.Close()

	m.mu.Lock()
	proc, ok := m.procs[taskID]
	if !ok || proc.cmd != cmd {
		m.mu.Unlock()
		return
	}

	now := time.Now()
	proc.logFile = nil
	proc.cmd = nil
	proc.done = nil
	proc.state.PID = 0
	proc.state.ExitedAt = &now
	m.stopHealthMonitorLocked(proc)

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	proc.state.ExitCode = &exitCode

	if proc.state.Status == StatusStopping {
		restartTaskID, delaySec, shouldRestart := m.finalizeStopLocked(proc)
		m.mu.Unlock()
		if shouldRestart {
			go m.scheduleRestart(restartTaskID, delaySec)
		}
		return
	}

	if err != nil {
		proc.state.Status = StatusExited
		proc.state.LastError = err.Error()
		restartTaskID, delaySec, shouldRestart := m.prepareRestartLocked(proc)
		m.mu.Unlock()
		if shouldRestart {
			go m.scheduleRestart(restartTaskID, delaySec)
		}
		return
	}

	proc.state.Status = StatusStopped
	proc.state.LastError = ""
	proc.restartCount = 0
	proc.healthFailures = 0
	m.mu.Unlock()
}

func (m *Manager) startHealthMonitorLocked(proc *managedProcess) {
	m.stopHealthMonitorLocked(proc)
	if proc.task.HealthCheckURL == "" {
		return
	}

	stopCh := make(chan struct{})
	proc.healthStop = stopCh
	proc.healthFailures = 0

	intervalSec := proc.task.HealthCheckIntervalSec
	if intervalSec <= 0 {
		intervalSec = 10
	}
	failureThreshold := proc.task.HealthCheckFailureThreshold
	if failureThreshold <= 0 {
		failureThreshold = 3
	}

	go m.runHealthMonitor(proc.task.ID, proc.task.HealthCheckURL, intervalSec, failureThreshold, stopCh)
}

func (m *Manager) stopHealthMonitorLocked(proc *managedProcess) {
	if proc.healthStop != nil {
		close(proc.healthStop)
		proc.healthStop = nil
	}
	proc.healthFailures = 0
}

func (m *Manager) runHealthMonitor(taskID string, healthURL string, intervalSec int, failureThreshold int, stopCh chan struct{}) {
	timeout := 5 * time.Second
	if intervalSec > 0 && time.Duration(intervalSec)*time.Second < timeout {
		timeout = time.Duration(intervalSec) * time.Second
	}
	client := &http.Client{Timeout: timeout}
	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			if err := performHealthCheck(client, healthURL); err != nil {
				message := fmt.Sprintf("health check failed: %v", err)
				if m.recordHealthFailure(taskID, stopCh, message, failureThreshold) {
					_ = m.stopWithReason(taskID, stopReasonHealth, message, stopCh)
					return
				}
				continue
			}
			m.recordHealthSuccess(taskID, stopCh)
		}
	}
}

func performHealthCheck(client *http.Client, healthURL string) error {
	req, err := http.NewRequest(http.MethodGet, healthURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func (m *Manager) recordHealthFailure(taskID string, stopCh chan struct{}, message string, threshold int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	proc, ok := m.procs[taskID]
	if !ok || proc.healthStop != stopCh || !isRunningStatus(proc.state.Status) {
		return false
	}

	proc.healthFailures++
	proc.state.LastError = message
	return proc.healthFailures >= threshold
}

func (m *Manager) recordHealthSuccess(taskID string, stopCh chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	proc, ok := m.procs[taskID]
	if !ok || proc.healthStop != stopCh || !isRunningStatus(proc.state.Status) {
		return
	}
	proc.healthFailures = 0
	if strings.HasPrefix(proc.state.LastError, "health check failed:") {
		proc.state.LastError = ""
	}
}

func (m *Manager) finalizeStopLocked(proc *managedProcess) (string, int, bool) {
	reason := proc.stopReason
	message := proc.stopMessage
	proc.stopReason = ""
	proc.stopMessage = ""
	proc.healthFailures = 0

	if reason == stopReasonHealth {
		proc.state.Status = StatusFailed
		proc.state.LastError = message
		return m.prepareRestartLocked(proc)
	}

	proc.state.Status = StatusStopped
	proc.state.LastError = ""
	proc.restartCount = 0
	return "", 0, false
}

func (m *Manager) prepareRestartLocked(proc *managedProcess) (string, int, bool) {
	delaySec := proc.task.RestartDelaySec
	if delaySec <= 0 {
		delaySec = 2
	}

	shouldRestart := proc.task.RestartOnCrash
	proc.restartCount++
	if proc.task.MaxRestartCount > 0 && proc.restartCount > proc.task.MaxRestartCount {
		shouldRestart = false
	}
	return proc.state.TaskID, delaySec, shouldRestart
}

func (m *Manager) scheduleRestart(taskID string, delaySec int) {
	time.Sleep(time.Duration(delaySec) * time.Second)
	_ = m.start(taskID, false)
}

// runBannerLabel picks the banner wording written at the top of each run.
func runBannerLabel(freshRun bool) string {
	if freshRun {
		return "运行开始"
	}
	return "自动重启"
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
