package app

import (
	"path/filepath"
	"testing"

	"tray/internal/process"
	"tray/internal/store"
	"tray/internal/task"
)

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
		ID:             "  demo  ",
		Name:           "  Demo Task  ",
		Program:        "  demo.exe  ",
		WorkDir:        "   ",
		StopTimeoutSec: 0,
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
}

func TestReplaceTasksRejectsDuplicateID(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{task.DefaultOpenListTask()})

	err := rt.ReplaceTasks([]task.Config{
		{ID: "same", Name: "A", Program: "a.exe"},
		{ID: "same", Name: "B", Program: "b.exe"},
	})
	if err == nil || err.Error() != `duplicate task id "same"` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReplaceTasksUsesDefaultWhenEmpty(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{task.DefaultOpenListTask()})

	if err := rt.ReplaceTasks(nil); err != nil {
		t.Fatalf("replace tasks: %v", err)
	}

	tasks := rt.ListTasks()
	if len(tasks) != 1 || tasks[0].ID != "openlist" {
		t.Fatalf("unexpected tasks after replace: %+v", tasks)
	}
}
