package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"zhulingtai/internal/task"
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

// TestTaskStoreSaveRollsBackOnLoopFailure guards the transaction rollback fix.
// The DELETE runs before the insert loop; an insert failure mid-loop must roll
// the whole transaction back so the committed baseline is preserved, the failed
// connection is returned to the pool, and a subsequent valid save still works.
// Before the fix, the loop-local err (declared with :=) shadowed the outer err
// that the deferred rollback checked, so mid-loop failures skipped rollback and
// leaked a connection per failed save.
func TestTaskStoreSaveRollsBackOnLoopFailure(t *testing.T) {
	dir := t.TempDir()
	store, err := NewTaskStore(filepath.Join(dir, "tasks.db"), filepath.Join(dir, "tasks.json"))
	if err != nil {
		t.Fatalf("new task store: %v", err)
	}
	defer store.Close()

	baseline := []task.Config{
		{ID: "keep", Name: "Keep", Program: "k.exe", Args: []string{}, WorkDir: ".", Env: []string{}, StopTimeoutSec: 8},
	}
	if err := store.Save(baseline); err != nil {
		t.Fatalf("baseline save: %v", err)
	}

	// Duplicate primary key triggers a stmt.Exec error after DELETE has run.
	dup := []task.Config{
		{ID: "x", Name: "X1", Program: "x.exe", Args: []string{}, WorkDir: ".", Env: []string{}, StopTimeoutSec: 8},
		{ID: "x", Name: "X2", Program: "x2.exe", Args: []string{}, WorkDir: ".", Env: []string{}, StopTimeoutSec: 8},
	}
	if err := store.Save(dup); err == nil {
		t.Fatal("expected duplicate-id save to fail, got nil")
	}

	// Rollback must restore the committed baseline unchanged.
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load after failed save: %v", err)
	}
	if len(loaded) != 1 || loaded[0].ID != "keep" {
		t.Fatalf("rollback did not restore baseline: %+v", loaded)
	}

	// Repeated failed saves must not leak connections: the fix rolls back and
	// returns each tx connection to the pool. The old code skipped rollback,
	// pinning one connection per failed save.
	before := store.db.Stats().OpenConnections
	for i := 0; i < 4; i++ {
		if saveErr := store.Save(dup); saveErr == nil {
			t.Fatalf("iteration %d: expected failure, got nil", i)
		}
	}
	after := store.db.Stats().OpenConnections
	if after > before+2 {
		t.Fatalf("failed saves leaked connections (rollback skipped): before=%d after=%d", before, after)
	}

	// After failure, a valid save must still succeed.
	next := []task.Config{
		{ID: "keep", Name: "Keep", Program: "k.exe", Args: []string{}, WorkDir: ".", Env: []string{}, StopTimeoutSec: 8},
		{ID: "more", Name: "More", Program: "m.exe", Args: []string{}, WorkDir: ".", Env: []string{}, StopTimeoutSec: 8},
	}
	if err := store.Save(next); err != nil {
		t.Fatalf("resave after failure: %v", err)
	}
	loaded, err = store.Load()
	if err != nil {
		t.Fatalf("reload after resave: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("resave state: %+v", loaded)
	}
}
