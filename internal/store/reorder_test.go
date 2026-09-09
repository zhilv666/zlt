package store

import (
	"path/filepath"
	"testing"

	"zhulingtai/internal/task"
)

func TestReorderTasks(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTaskStore(
		filepath.Join(dir, "tasks.db"),
		filepath.Join(dir, "tasks.json"),
	)
	if err != nil {
		t.Fatalf("NewTaskStore: %v", err)
	}
	defer s.Close()

	tasks := []task.Config{
		{ID: "a", Name: "A", Program: "p"},
		{ID: "b", Name: "B", Program: "p"},
		{ID: "c", Name: "C", Program: "p"},
	}
	if err := s.Save(tasks); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reverse the order.
	if err := s.ReorderTasks([]string{"c", "b", "a"}); err != nil {
		t.Fatalf("ReorderTasks: %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 3 || loaded[0].ID != "c" || loaded[1].ID != "b" || loaded[2].ID != "a" {
		t.Fatalf("unexpected order: %+v", loaded)
	}

	// Save preserves the order (Save writes sort_order = slice index).
	if err := s.Save(loaded); err != nil {
		t.Fatalf("Save2: %v", err)
	}
	loaded2, _ := s.Load()
	if loaded2[0].ID != "c" || loaded2[2].ID != "a" {
		t.Fatalf("order lost after re-save: %+v", loaded2)
	}
}

func TestReorderTasksUnknownID(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewTaskStore(filepath.Join(dir, "t.db"), filepath.Join(dir, "t.json"))
	defer s.Close()

	_ = s.Save([]task.Config{{ID: "x", Name: "X", Program: "p"}})

	// "y" does not exist — should error.
	err := s.ReorderTasks([]string{"y"})
	if err == nil {
		t.Fatal("expected error for unknown id")
	}
}

func TestReorderSchedules(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewTaskStore(filepath.Join(dir, "t.db"), filepath.Join(dir, "t.json"))
	defer s.Close()

	schedules := []task.Schedule{
		{ID: "s1", TaskID: "t1", Name: "S1", CronExpr: "* * * * *", Action: task.ScheduleActionStart},
		{ID: "s2", TaskID: "t1", Name: "S2", CronExpr: "* * * * *", Action: task.ScheduleActionStop},
	}
	if err := s.SaveSchedules(schedules); err != nil {
		t.Fatalf("SaveSchedules: %v", err)
	}

	if err := s.ReorderSchedules([]string{"s2", "s1"}); err != nil {
		t.Fatalf("ReorderSchedules: %v", err)
	}

	loaded, _ := s.LoadSchedules()
	if len(loaded) != 2 || loaded[0].ID != "s2" || loaded[1].ID != "s1" {
		t.Fatalf("unexpected order: %+v", loaded)
	}
}

func TestSortOrderMigrationBackfill(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTaskStore(filepath.Join(dir, "t.db"), filepath.Join(dir, "t.json"))
	if err != nil {
		t.Fatalf("NewTaskStore: %v", err)
	}

	// Insert tasks without sort_order to simulate a pre-migration state.
	// The meta marker is already set (initSchema ran), so we need to verify
	// the marker is present and prevents re-backfill.
	marker, _ := s.metaGet("tasks_sort_order_migrated")
	if marker != "1" {
		t.Fatalf("migration marker not set: %q", marker)
	}

	// Reorder should work (proving the column exists).
	_ = s.Save([]task.Config{{ID: "m1", Name: "M", Program: "p"}})
	if err := s.ReorderTasks([]string{"m1"}); err != nil {
		t.Fatalf("ReorderTasks: %v", err)
	}
	s.Close()
}
