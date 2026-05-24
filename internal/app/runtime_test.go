package app

import (
	"errors"
	"path/filepath"
	"testing"

	"zhulingtai/internal/process"
	"zhulingtai/internal/store"
	"zhulingtai/internal/task"
)

func newDummyTask() task.Config {
	return task.Config{
		ID:      "dummy",
		Name:    "Dummy",
		Program: "__definitely_not_a_real_process__",
		Args:    []string{},
		WorkDir: ".",
		Env:     []string{},
	}
}

func newTestRuntime(t *testing.T, tasks []task.Config) *Runtime {
	t.Helper()

	dir := t.TempDir()
	taskStore, err := store.NewTaskStore(
		filepath.Join(dir, "tasks.db"),
		filepath.Join(dir, "tasks.json"),
	)
	if err != nil {
		t.Fatalf("create task store: %v", err)
	}
	t.Cleanup(func() {
		_ = taskStore.Close()
	})

	if err := taskStore.Save(tasks); err != nil {
		t.Fatalf("seed task store: %v", err)
	}

	return &Runtime{
		TaskStore: taskStore,
		Tasks:     tasks,
		Manager:   process.NewManager(tasks),
	}
}

func TestNormalizeTask(t *testing.T) {
	cfg := normalizeTask(task.Config{
		ID:                     "  demo  ",
		Name:                   "  Demo Task  ",
		Program:                "  demo.exe  ",
		WorkDir:                "   ",
		StopTimeoutSec:         0,
		HealthCheckURL:         "  http://127.0.0.1:8080/health  ",
		HealthCheckIntervalSec: 0,
	})

	if cfg.ID != "demo" || cfg.Name != "Demo Task" || cfg.Program != "demo.exe" {
		t.Fatalf("unexpected normalized fields: %+v", cfg)
	}
	if cfg.WorkDir != "." {
		t.Fatalf("expected default workdir, got %q", cfg.WorkDir)
	}
	if cfg.StopTimeoutSec != 8 {
		t.Fatalf("expected default timeout, got %d", cfg.StopTimeoutSec)
	}
	if cfg.RestartDelaySec != 2 {
		t.Fatalf("expected default restart delay, got %d", cfg.RestartDelaySec)
	}
	if cfg.MaxRestartCount != 0 {
		t.Fatalf("expected default max restart count, got %d", cfg.MaxRestartCount)
	}
	if cfg.HealthCheckURL != "http://127.0.0.1:8080/health" {
		t.Fatalf("expected trimmed health check url, got %q", cfg.HealthCheckURL)
	}
	if cfg.HealthCheckIntervalSec != 10 {
		t.Fatalf("expected default health check interval, got %d", cfg.HealthCheckIntervalSec)
	}
	if cfg.HealthCheckFailureThreshold != 3 {
		t.Fatalf("expected default health check threshold, got %d", cfg.HealthCheckFailureThreshold)
	}
	if cfg.Args == nil || cfg.Env == nil {
		t.Fatalf("expected initialized slices")
	}
}

func TestValidateTask(t *testing.T) {
	cases := []struct {
		name string
		cfg  task.Config
		want string
	}{
		{name: "missing id", cfg: task.Config{Name: "n", Program: "p"}, want: "id is required"},
		{name: "missing name", cfg: task.Config{ID: "i", Program: "p"}, want: "name is required"},
		{name: "missing program", cfg: task.Config{ID: "i", Name: "n"}, want: "program is required"},
		{name: "negative restart delay", cfg: task.Config{ID: "i", Name: "n", Program: "p", RestartDelaySec: -1}, want: "restart delay must be >= 0"},
		{name: "negative max restart", cfg: task.Config{ID: "i", Name: "n", Program: "p", MaxRestartCount: -1}, want: "max restart count must be >= 0"},
		{name: "negative health interval", cfg: task.Config{ID: "i", Name: "n", Program: "p", HealthCheckIntervalSec: -1}, want: "health check interval must be >= 0"},
		{name: "negative health threshold", cfg: task.Config{ID: "i", Name: "n", Program: "p", HealthCheckFailureThreshold: -1}, want: "health check failure threshold must be >= 0"},
		{name: "invalid health url", cfg: task.Config{ID: "i", Name: "n", Program: "p", HealthCheckURL: "tcp://demo", HealthCheckIntervalSec: 10, HealthCheckFailureThreshold: 3}, want: "health check URL must be a valid http/https URL"},
		{name: "missing health interval when enabled", cfg: task.Config{ID: "i", Name: "n", Program: "p", HealthCheckURL: "http://127.0.0.1/health", HealthCheckFailureThreshold: 3}, want: "health check interval must be > 0 when health check is enabled"},
		{name: "missing health threshold when enabled", cfg: task.Config{ID: "i", Name: "n", Program: "p", HealthCheckURL: "http://127.0.0.1/health", HealthCheckIntervalSec: 10}, want: "health check failure threshold must be > 0 when health check is enabled"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTask(tc.cfg)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}

	if err := validateTask(task.Config{ID: "demo", Name: "Demo", Program: "demo.exe"}); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if err := validateTask(task.Config{
		ID:                          "demo-http",
		Name:                        "Demo HTTP",
		Program:                     "demo.exe",
		HealthCheckURL:              "https://127.0.0.1/health",
		HealthCheckIntervalSec:      15,
		HealthCheckFailureThreshold: 4,
	}); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestReplaceTasksRejectsDuplicateID(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{newDummyTask()})

	err := rt.ReplaceTasks([]task.Config{
		{ID: "same", Name: "A", Program: "a.exe"},
		{ID: "same", Name: "B", Program: "b.exe"},
	})
	if err == nil || err.Error() != `duplicate task id "same"` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReplaceTasksKeepsEmptyWhenEmpty(t *testing.T) {
	rt := newTestRuntime(t, nil)

	if err := rt.ReplaceTasks(nil); err != nil {
		t.Fatalf("replace tasks: %v", err)
	}

	tasks := rt.ListTasks()
	if len(tasks) != 0 {
		t.Fatalf("unexpected tasks after replace: %+v", tasks)
	}
}

func TestRestartTaskReturnsNotFound(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{newDummyTask()})

	err := rt.RestartTask("missing")
	if err == nil || !errors.Is(err, process.ErrTaskNotFound) {
		t.Fatalf("expected task not found error, got %v", err)
	}
}

func TestSetRestartOnCrashPersists(t *testing.T) {
	cfg := task.Config{
		ID:             "demo",
		Name:           "Demo",
		Program:        "demo.exe",
		RestartOnCrash: false,
	}
	rt := newTestRuntime(t, []task.Config{cfg})

	if err := rt.SetRestartOnCrash(cfg.ID, true); err != nil {
		t.Fatalf("set restart on crash: %v", err)
	}

	tasks := rt.ListTasks()
	if len(tasks) != 1 || !tasks[0].RestartOnCrash {
		t.Fatalf("unexpected runtime tasks: %+v", tasks)
	}

	loaded, err := rt.TaskStore.Load()
	if err != nil {
		t.Fatalf("reload tasks: %v", err)
	}
	if len(loaded) != 1 || !loaded[0].RestartOnCrash {
		t.Fatalf("unexpected persisted tasks: %+v", loaded)
	}
}
