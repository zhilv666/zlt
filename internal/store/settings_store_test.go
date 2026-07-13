package store

import (
	"path/filepath"
	"testing"

	"zhulingtai/internal/task"
)

func TestSettingsDefaultsThenRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewTaskStore(filepath.Join(dir, "tasks.db"), filepath.Join(dir, "tasks.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	// A fresh store reports the built-in defaults.
	got, err := store.LoadSettings()
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	if got != task.DefaultSettings() {
		t.Fatalf("defaults = %+v, want %+v", got, task.DefaultSettings())
	}

	// Save then reload round-trips exactly.
	want := task.Settings{
		LogLevel:          "warn",
		AppLogMaxSizeMB:   20,
		AppLogMaxBackups:  5,
		TaskLogMaxSizeMB:  50,
		TaskLogMaxBackups: 2,
	}
	if err := store.SaveSettings(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err = store.LoadSettings()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got != want {
		t.Fatalf("reloaded = %+v, want %+v", got, want)
	}

	// Saving again updates the same single row (no duplicate-row error).
	want.LogLevel = "error"
	if err := store.SaveSettings(want); err != nil {
		t.Fatalf("second save: %v", err)
	}
	got, _ = store.LoadSettings()
	if got.LogLevel != "error" {
		t.Fatalf("update not applied: %+v", got)
	}
}
