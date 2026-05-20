package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"tray/internal/task"
)

func TestTaskStoreSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := NewTaskStore(filepath.Join(dir, "tasks.db"), filepath.Join(dir, "tasks.json"))
	if err != nil {
		t.Fatalf("new task store: %v", err)
	}
	defer store.Close()

	input := []task.Config{
		{
			ID:                          "openlist",
			Name:                        "OpenList",
			Program:                     "openlist.exe",
			Args:                        []string{"server"},
			WorkDir:                     "D:/SoftWare/OpenList",
			Env:                         []string{"A=1"},
			AutoStart:                   true,
			RestartOnCrash:              true,
			StopTimeoutSec:              12,
			RestartDelaySec:             5,
			MaxRestartCount:             9,
			HealthCheckURL:              "http://127.0.0.1:5244/health",
			HealthCheckIntervalSec:      11,
			HealthCheckFailureThreshold: 4,
		},
	}

	if err := store.Save(input); err != nil {
		t.Fatalf("save tasks: %v", err)
	}

	output, err := store.Load()
	if err != nil {
		t.Fatalf("load tasks: %v", err)
	}
	if len(output) != 1 {
		t.Fatalf("unexpected task count: %d", len(output))
	}
	got := output[0]
	if got.ID != input[0].ID || got.Program != input[0].Program || got.WorkDir != input[0].WorkDir {
		t.Fatalf("unexpected loaded task: %+v", got)
	}
	if !got.AutoStart || !got.RestartOnCrash || got.StopTimeoutSec != 12 || got.RestartDelaySec != 5 || got.MaxRestartCount != 9 {
		t.Fatalf("unexpected loaded flags: %+v", got)
	}
	if got.HealthCheckURL != "http://127.0.0.1:5244/health" || got.HealthCheckIntervalSec != 11 || got.HealthCheckFailureThreshold != 4 {
		t.Fatalf("unexpected health check config: %+v", got)
	}
	if len(got.Args) != 1 || got.Args[0] != "server" || len(got.Env) != 1 || got.Env[0] != "A=1" {
		t.Fatalf("unexpected loaded arrays: %+v", got)
	}
}

func TestTaskStoreBootstrapsFromLegacyJSON(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "tasks.json")
	legacy := []task.Config{
		{ID: "demo", Name: "Demo", Program: "demo.exe", Args: []string{"run"}, WorkDir: ".", Env: []string{}, StopTimeoutSec: 8},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		t.Fatalf("write legacy json: %v", err)
	}

	store, err := NewTaskStore(filepath.Join(dir, "tasks.db"), jsonPath)
	if err != nil {
		t.Fatalf("new task store: %v", err)
	}
	defer store.Close()

	tasks, err := store.Load()
	if err != nil {
		t.Fatalf("load tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "demo" {
		t.Fatalf("unexpected bootstrapped tasks: %+v", tasks)
	}
}
