package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"tray/internal/process"
	"tray/internal/store"
	"tray/internal/task"
)

type Runtime struct {
	mu        sync.RWMutex
	TaskStore *store.TaskStore
	Tasks     []task.Config
	Manager   *process.Manager
	HTTP      *http.Server
}

func NewRuntime() (*Runtime, error) {
	taskStore := store.NewTaskStore(filepath.Join("data", "tasks.json"))
	tasks, err := taskStore.Load()
	if err != nil {
		return nil, err
	}
	if err := taskStore.Save(tasks); err != nil {
		return nil, err
	}

	manager := process.NewManager(tasks)

	return &Runtime{
		TaskStore: taskStore,
		Tasks:     tasks,
		Manager:   manager,
	}, nil
}

func (r *Runtime) StartHTTP() error {
	go func() {
		_ = r.HTTP.ListenAndServe()
	}()
	return nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	return r.HTTP.Shutdown(ctx)
}

func (r *Runtime) Address() string {
	return fmt.Sprintf("http://%s", r.HTTP.Addr)
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

	if err := r.Manager.RemoveTask(taskID); err != nil {
		return err
	}

	r.Tasks = append(r.Tasks[:index], r.Tasks[index+1:]...)
	return r.TaskStore.Save(r.Tasks)
}

func normalizeTask(cfg task.Config) task.Config {
	cfg.ID = strings.TrimSpace(cfg.ID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Program = strings.TrimSpace(cfg.Program)
	cfg.WorkDir = strings.TrimSpace(cfg.WorkDir)
	if cfg.WorkDir == "" {
		cfg.WorkDir = "."
	}
	if cfg.StopTimeoutSec <= 0 {
		cfg.StopTimeoutSec = 8
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
	return nil
}
