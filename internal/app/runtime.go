package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"zhulingtai/internal/process"
	"zhulingtai/internal/scheduler"
	"zhulingtai/internal/store"
	"zhulingtai/internal/task"
)

type Runtime struct {
	mu        sync.RWMutex
	TaskStore *store.TaskStore
	Tasks     []task.Config
	Schedules []task.Schedule
	Manager   *process.Manager
	Sched     *scheduler.Scheduler
	HTTP      *http.Server

	autoStartOnce sync.Once
	autoStartErr  error
	shutdownOnce  sync.Once
	shutdownErr   error
}

func NewRuntime() (*Runtime, error) {
	taskStore, err := store.NewTaskStore(
		filepath.Join("data", "tasks.db"),
		filepath.Join("data", "tasks.json"),
	)
	if err != nil {
		return nil, err
	}
	tasks, err := taskStore.Load()
	if err != nil {
		return nil, err
	}
	if err := taskStore.Save(tasks); err != nil {
		return nil, err
	}
	schedules, err := taskStore.LoadSchedules()
	if err != nil {
		return nil, err
	}

	manager := process.NewManager(tasks)

	runtime := &Runtime{
		TaskStore: taskStore,
		Tasks:     tasks,
		Schedules: schedules,
		Manager:   manager,
	}
	runtime.Sched = scheduler.New(runtime, runtime.recordScheduleResult)

	return runtime, nil
}

func (r *Runtime) StartHTTP() error {
	go func() {
		_ = r.HTTP.ListenAndServe()
	}()
	return nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	r.shutdownOnce.Do(func() {
		// Stop the scheduler first so no cron job fires new task operations
		// while tasks are being stopped below.
		if r.Sched != nil {
			r.Sched.Stop(scheduleStopTimeout)
		}
		if err := r.StopAllTasks(); err != nil {
			r.shutdownErr = err
		}
		if r.HTTP != nil {
			shutdownCtx := ctx
			cancel := func() {}
			if _, ok := ctx.Deadline(); !ok {
				shutdownCtx, cancel = context.WithTimeout(ctx, 3*time.Second)
			}
			defer cancel()

			if err := r.HTTP.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("runtime shutdown: http shutdown err=%v, forcing close", err)
				if closeErr := r.HTTP.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) && r.shutdownErr == nil {
					r.shutdownErr = closeErr
				}
				if r.shutdownErr == nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
					r.shutdownErr = err
				}
			}
		}
		if r.TaskStore != nil {
			_ = r.TaskStore.Close()
		}
	})
	return r.shutdownErr
}

func (r *Runtime) Address() string {
	return fmt.Sprintf("http://%s", r.HTTP.Addr)
}

func (r *Runtime) States() []process.RuntimeState {
	return r.Manager.States()
}

func (r *Runtime) State(taskID string) (process.RuntimeState, bool) {
	return r.Manager.State(taskID)
}

func (r *Runtime) Start(taskID string) error {
	return r.Manager.Start(taskID)
}

func (r *Runtime) Stop(taskID string) error {
	return r.Manager.Stop(taskID)
}

func (r *Runtime) ClearLogs(taskID string) error {
	return r.Manager.ClearLogs(taskID)
}

func (r *Runtime) ListTasks() []task.Config {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]task.Config, len(r.Tasks))
	copy(out, r.Tasks)
	return out
}

func (r *Runtime) UpsertTask(cfg task.Config) error {
	cfg = normalizeTask(cfg)
	if err := validateTask(cfg); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.Tasks {
		if r.Tasks[i].ID == cfg.ID {
			state, ok := r.Manager.State(cfg.ID)
			if ok && state.Status != process.StatusStopped && state.Status != process.StatusExited && state.Status != process.StatusFailed {
				return errors.New("task is running, stop it before editing")
			}
			r.Tasks[i] = cfg
			r.Manager.UpsertTask(cfg)
			return r.TaskStore.Save(r.Tasks)
		}
	}

	r.Tasks = append(r.Tasks, cfg)
	r.Manager.UpsertTask(cfg)
	return r.TaskStore.Save(r.Tasks)
}

func (r *Runtime) DeleteTask(taskID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	index := -1
	for i := range r.Tasks {
		if r.Tasks[i].ID == taskID {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("task %q not found", taskID)
	}

	if n := r.countSchedulesForTaskLocked(taskID); n > 0 {
		return fmt.Errorf("task %q has %d schedule(s) attached, delete them first", taskID, n)
	}

	if err := r.Manager.RemoveTask(taskID); err != nil {
		return err
	}

	r.Tasks = append(r.Tasks[:index], r.Tasks[index+1:]...)
	return r.TaskStore.Save(r.Tasks)
}

func (r *Runtime) ExportTasks() []task.Config {
	return r.ListTasks()
}

func (r *Runtime) ReplaceTasks(tasks []task.Config) error {
	normalized := make([]task.Config, 0, len(tasks))
	seen := make(map[string]struct{}, len(tasks))
	for _, cfg := range tasks {
		cfg = normalizeTask(cfg)
		if err := validateTask(cfg); err != nil {
			return err
		}
		if _, exists := seen[cfg.ID]; exists {
			return fmt.Errorf("duplicate task id %q", cfg.ID)
		}
		seen[cfg.ID] = struct{}{}
		normalized = append(normalized, cfg)
	}

	for _, st := range r.Manager.States() {
		if st.Status == process.StatusRunning || st.Status == process.StatusStarting || st.Status == process.StatusStopping {
			return errors.New("stop all running tasks before import")
		}
	}

	r.mu.Lock()
	r.Tasks = normalized
	r.Manager = process.NewManager(normalized)
	if err := r.TaskStore.Save(r.Tasks); err != nil {
		r.mu.Unlock()
		return err
	}
	// Import may have removed tasks that schedules reference; keep those
	// schedules but disable them so the scheduler never fires into a void.
	if err := r.disableOrphanSchedulesLocked(); err != nil {
		r.mu.Unlock()
		return err
	}
	r.reloadSchedulerLocked()
	r.mu.Unlock()
	return r.applyAutoStart()
}

func normalizeTask(cfg task.Config) task.Config {
	cfg.ID = strings.TrimSpace(cfg.ID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Program = strings.TrimSpace(cfg.Program)
	cfg.WorkDir = strings.TrimSpace(cfg.WorkDir)
	cfg.HealthCheckURL = strings.TrimSpace(cfg.HealthCheckURL)
	if cfg.WorkDir == "" {
		cfg.WorkDir = "."
	}
	if cfg.StopTimeoutSec <= 0 {
		cfg.StopTimeoutSec = 8
	}
	if cfg.RestartDelaySec <= 0 {
		cfg.RestartDelaySec = 2
	}
	if cfg.MaxRestartCount < 0 {
		cfg.MaxRestartCount = 0
	}
	if cfg.HealthCheckURL != "" {
		if cfg.HealthCheckIntervalSec <= 0 {
			cfg.HealthCheckIntervalSec = 10
		}
		if cfg.HealthCheckFailureThreshold <= 0 {
			cfg.HealthCheckFailureThreshold = 3
		}
	}
	if cfg.Args == nil {
		cfg.Args = []string{}
	}
	if cfg.Env == nil {
		cfg.Env = []string{}
	}
	return cfg
}

func validateTask(cfg task.Config) error {
	if cfg.ID == "" {
		return errors.New("id is required")
	}
	if cfg.Name == "" {
		return errors.New("name is required")
	}
	if cfg.Program == "" {
		return errors.New("program is required")
	}
	if cfg.RestartDelaySec < 0 {
		return errors.New("restart delay must be >= 0")
	}
	if cfg.MaxRestartCount < 0 {
		return errors.New("max restart count must be >= 0")
	}
	if cfg.HealthCheckIntervalSec < 0 {
		return errors.New("health check interval must be >= 0")
	}
	if cfg.HealthCheckFailureThreshold < 0 {
		return errors.New("health check failure threshold must be >= 0")
	}
	for _, entry := range cfg.Env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			return fmt.Errorf("environment variable must be in KEY=VALUE form: %q", entry)
		}
		if key == "" {
			return fmt.Errorf("environment variable key must not be empty: %q", entry)
		}
		if strings.ContainsRune(entry, 0) {
			return fmt.Errorf("environment variable must not contain NUL: %q", entry)
		}
	}
	if cfg.HealthCheckURL != "" {
		parsed, err := url.ParseRequestURI(cfg.HealthCheckURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errors.New("health check URL must be a valid http/https URL")
		}
		if cfg.HealthCheckIntervalSec <= 0 {
			return errors.New("health check interval must be > 0 when health check is enabled")
		}
		if cfg.HealthCheckFailureThreshold <= 0 {
			return errors.New("health check failure threshold must be > 0 when health check is enabled")
		}
	}
	return nil
}

func (r *Runtime) applyAutoStart() error {
	for _, cfg := range r.ListTasks() {
		if !cfg.AutoStart {
			continue
		}
		log.Printf("autostart task: starting %s", cfg.ID)
		if err := r.Manager.Start(cfg.ID); err != nil {
			log.Printf("autostart task: failed %s err=%v", cfg.ID, err)
			return fmt.Errorf("auto-start %s failed: %w", cfg.ID, err)
		}
		log.Printf("autostart task: started %s", cfg.ID)
	}
	return nil
}

func (r *Runtime) StartAutoStartTasks() error {
	r.autoStartOnce.Do(func() {
		r.autoStartErr = r.applyAutoStart()
	})
	return r.autoStartErr
}

func (r *Runtime) StopAllTasks() error {
	var firstErr error
	for _, cfg := range r.ListTasks() {
		log.Printf("shutdown task: stopping %s", cfg.ID)
		if err := r.Manager.Stop(cfg.ID); err != nil && firstErr == nil {
			log.Printf("shutdown task: failed %s err=%v", cfg.ID, err)
			firstErr = err
			continue
		}
		log.Printf("shutdown task: stopped %s", cfg.ID)
	}
	return firstErr
}

func (r *Runtime) RestartTask(taskID string) error {
	state, ok := r.Manager.State(taskID)
	if !ok {
		return fmt.Errorf("%w: %s", process.ErrTaskNotFound, taskID)
	}
	if state.Status == process.StatusRunning || state.Status == process.StatusStarting || state.Status == process.StatusStopping {
		if err := r.Manager.Stop(taskID); err != nil {
			return err
		}
		for i := 0; i < 50; i++ {
			state, ok = r.Manager.State(taskID)
			if !ok {
				return fmt.Errorf("%w: %s", process.ErrTaskNotFound, taskID)
			}
			if state.Status != process.StatusRunning && state.Status != process.StatusStarting && state.Status != process.StatusStopping {
				return r.Manager.Start(taskID)
			}
			time.Sleep(100 * time.Millisecond)
		}
		return errors.New("task did not stop in time before restart")
	}
	return r.Manager.Start(taskID)
}

func (r *Runtime) SetRestartOnCrash(taskID string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.Tasks {
		if r.Tasks[i].ID != taskID {
			continue
		}

		r.Tasks[i].RestartOnCrash = enabled
		r.Manager.UpsertTask(r.Tasks[i])
		return r.TaskStore.Save(r.Tasks)
	}

	return fmt.Errorf("%w: %s", process.ErrTaskNotFound, taskID)
}
