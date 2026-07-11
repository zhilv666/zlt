package store

import (
	"path/filepath"
	"testing"

	"zhulingtai/internal/task"
)

func TestScheduleStoreSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := NewTaskStore(filepath.Join(dir, "tasks.db"), filepath.Join(dir, "tasks.json"))
	if err != nil {
		t.Fatalf("new task store: %v", err)
	}
	defer store.Close()

	input := []task.Schedule{
		{
			ID:         "sch-morning",
			TaskID:     "openlist",
			Name:       "工作日启动",
			CronExpr:   "0 8 * * 1-5",
			Timezone:   "Asia/Shanghai",
			Action:     task.ScheduleActionStart,
			Enabled:    true,
			LastRunAt:  "2026-07-11T08:00:00+08:00",
			LastStatus: task.ScheduleRunSuccess,
			LastDetail: "",
			CreatedAt:  "2026-07-10T20:00:00+08:00",
			UpdatedAt:  "2026-07-10T21:00:00+08:00",
		},
		{
			ID:       "sch-night",
			TaskID:   "openlist",
			Name:     "每晚停止",
			CronExpr: "0 23 * * *",
			Action:   task.ScheduleActionStop,
			Enabled:  false,
		},
	}

	if err := store.SaveSchedules(input); err != nil {
		t.Fatalf("save schedules: %v", err)
	}

	output, err := store.LoadSchedules()
	if err != nil {
		t.Fatalf("load schedules: %v", err)
	}
	if len(output) != 2 {
		t.Fatalf("unexpected schedule count: %d", len(output))
	}

	got := output[0]
	if got.ID != "sch-morning" || got.TaskID != "openlist" || got.CronExpr != "0 8 * * 1-5" || got.Timezone != "Asia/Shanghai" {
		t.Fatalf("unexpected loaded schedule: %+v", got)
	}
	if got.Action != task.ScheduleActionStart || !got.Enabled {
		t.Fatalf("unexpected action/enabled: %+v", got)
	}
	if got.LastRunAt != "2026-07-11T08:00:00+08:00" || got.LastStatus != task.ScheduleRunSuccess {
		t.Fatalf("unexpected last run fields: %+v", got)
	}
	if got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Fatalf("timestamps not persisted: %+v", got)
	}

	if output[1].Enabled || output[1].Action != task.ScheduleActionStop {
		t.Fatalf("unexpected second schedule: %+v", output[1])
	}

	// Full-replace semantics: saving a shorter list removes stale rows.
	if err := store.SaveSchedules(input[:1]); err != nil {
		t.Fatalf("resave schedules: %v", err)
	}
	output, err = store.LoadSchedules()
	if err != nil {
		t.Fatalf("reload schedules: %v", err)
	}
	if len(output) != 1 || output[0].ID != "sch-morning" {
		t.Fatalf("full replace failed: %+v", output)
	}
}

// Saving tasks must never disturb schedules even though TaskStore.Save uses
// DELETE + re-insert on the tasks table (this is why schedules carry no FK).
func TestScheduleSurvivesTaskSave(t *testing.T) {
	dir := t.TempDir()
	store, err := NewTaskStore(filepath.Join(dir, "tasks.db"), filepath.Join(dir, "tasks.json"))
	if err != nil {
		t.Fatalf("new task store: %v", err)
	}
	defer store.Close()

	if err := store.SaveSchedules([]task.Schedule{{
		ID: "sch-1", TaskID: "demo", Name: "n", CronExpr: "* * * * *", Action: task.ScheduleActionStart, Enabled: true,
	}}); err != nil {
		t.Fatalf("save schedules: %v", err)
	}

	tasks := []task.Config{{ID: "demo", Name: "Demo", Program: "demo.exe", Args: []string{}, WorkDir: ".", Env: []string{}, StopTimeoutSec: 8}}
	if err := store.Save(tasks); err != nil {
		t.Fatalf("save tasks: %v", err)
	}
	if err := store.Save(nil); err != nil {
		t.Fatalf("save empty tasks: %v", err)
	}

	schedules, err := store.LoadSchedules()
	if err != nil {
		t.Fatalf("load schedules: %v", err)
	}
	if len(schedules) != 1 || schedules[0].ID != "sch-1" {
		t.Fatalf("schedules were disturbed by task save: %+v", schedules)
	}
}
