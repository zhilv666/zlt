package app

import (
	"strings"
	"testing"
	"time"

	"zhulingtai/internal/task"
)

func newDummySchedule(taskID string) task.Schedule {
	return task.Schedule{
		ID:       "sch-test",
		TaskID:   taskID,
		Name:     "test schedule",
		CronExpr: "0 8 * * 1-5",
		Action:   task.ScheduleActionStart,
		Enabled:  true,
	}
}

func TestUpsertScheduleCreateAndPersist(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{newDummyTask()})

	sch := newDummySchedule("dummy")
	sch.ID = "" // exercise auto-generated IDs
	sch.Name = ""
	if err := rt.UpsertSchedule(sch); err != nil {
		t.Fatalf("upsert schedule: %v", err)
	}

	list := rt.ListSchedules()
	if len(list) != 1 {
		t.Fatalf("unexpected schedule count: %d", len(list))
	}
	got := list[0]
	if !strings.HasPrefix(got.ID, "sch-") {
		t.Fatalf("expected generated id, got %q", got.ID)
	}
	if got.Name != got.ID {
		t.Fatalf("expected name to default to id, got %q", got.Name)
	}
	if got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Fatalf("expected timestamps: %+v", got)
	}

	persisted, err := rt.TaskStore.LoadSchedules()
	if err != nil {
		t.Fatalf("load persisted schedules: %v", err)
	}
	if len(persisted) != 1 || persisted[0].ID != got.ID {
		t.Fatalf("schedule not persisted: %+v", persisted)
	}
}

func TestUpsertScheduleValidation(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{newDummyTask()})

	bad := newDummySchedule("dummy")
	bad.CronExpr = "not a cron"
	if err := rt.UpsertSchedule(bad); err == nil {
		t.Fatalf("expected invalid cron expression to be rejected")
	}

	orphan := newDummySchedule("missing-task")
	if err := rt.UpsertSchedule(orphan); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected unknown task to be rejected, got %v", err)
	}

	badTZ := newDummySchedule("dummy")
	badTZ.Timezone = "Mars/Olympus"
	if err := rt.UpsertSchedule(badTZ); err == nil {
		t.Fatalf("expected invalid timezone to be rejected")
	}
}

func TestUpsertScheduleUpdatePreservesHistory(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{newDummyTask()})

	sch := newDummySchedule("dummy")
	if err := rt.UpsertSchedule(sch); err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	created := rt.ListSchedules()[0]

	// Simulate a recorded run, then edit the schedule.
	rt.recordScheduleResult(created.ID, time.Now(), task.ScheduleRunSuccess, "")

	edit := newDummySchedule("dummy")
	edit.ID = created.ID
	edit.CronExpr = "*/10 * * * *"
	edit.LastStatus = "spoofed" // client-sent history must be ignored
	if err := rt.UpsertSchedule(edit); err != nil {
		t.Fatalf("update schedule: %v", err)
	}

	got := rt.ListSchedules()[0]
	if got.CronExpr != "*/10 * * * *" {
		t.Fatalf("expression not updated: %+v", got)
	}
	if got.CreatedAt != created.CreatedAt {
		t.Fatalf("created_at changed on update: %+v", got)
	}
	if got.LastStatus != task.ScheduleRunSuccess || got.LastRunAt == "" {
		t.Fatalf("run history lost on update: %+v", got)
	}
}

func TestDeleteScheduleAndNotFound(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{newDummyTask()})

	if err := rt.UpsertSchedule(newDummySchedule("dummy")); err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if err := rt.DeleteSchedule("sch-test"); err != nil {
		t.Fatalf("delete schedule: %v", err)
	}
	if len(rt.ListSchedules()) != 0 {
		t.Fatalf("schedule not removed")
	}
	if err := rt.DeleteSchedule("sch-test"); err == nil {
		t.Fatalf("expected not found error")
	}
}

func TestDeleteTaskRefusedWhileScheduleAttached(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{newDummyTask()})

	if err := rt.UpsertSchedule(newDummySchedule("dummy")); err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	err := rt.DeleteTask("dummy")
	if err == nil || !strings.Contains(err.Error(), "schedule") {
		t.Fatalf("expected refusal while schedule attached, got %v", err)
	}

	if err := rt.DeleteSchedule("sch-test"); err != nil {
		t.Fatalf("delete schedule: %v", err)
	}
	if err := rt.DeleteTask("dummy"); err != nil {
		t.Fatalf("delete task after removing schedule: %v", err)
	}
}

func TestSetScheduleEnabled(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{newDummyTask()})

	if err := rt.UpsertSchedule(newDummySchedule("dummy")); err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if err := rt.SetScheduleEnabled("sch-test", false); err != nil {
		t.Fatalf("disable schedule: %v", err)
	}
	if rt.ListSchedules()[0].Enabled {
		t.Fatalf("schedule still enabled")
	}
	if err := rt.SetScheduleEnabled("sch-test", true); err != nil {
		t.Fatalf("enable schedule: %v", err)
	}
	if !rt.ListSchedules()[0].Enabled {
		t.Fatalf("schedule still disabled")
	}
	if err := rt.SetScheduleEnabled("missing", true); err == nil {
		t.Fatalf("expected not found error")
	}
}

func TestReplaceTasksDisablesOrphanSchedules(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{newDummyTask()})

	if err := rt.UpsertSchedule(newDummySchedule("dummy")); err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	// Import a task set that no longer contains "dummy".
	if err := rt.ReplaceTasks([]task.Config{{ID: "other", Name: "Other", Program: "other.exe"}}); err != nil {
		t.Fatalf("replace tasks: %v", err)
	}

	got := rt.ListSchedules()[0]
	if got.Enabled {
		t.Fatalf("orphan schedule should be disabled: %+v", got)
	}
	if !strings.Contains(got.LastDetail, "task not found") {
		t.Fatalf("orphan reason not recorded: %+v", got)
	}

	// Re-enabling an orphan must be refused.
	if err := rt.SetScheduleEnabled("sch-test", true); err == nil {
		t.Fatalf("expected orphan enable to be refused")
	}

	// Manual run of the orphan reports failed and records the result.
	result, err := rt.RunScheduleNow("sch-test")
	if err != nil {
		t.Fatalf("run schedule now: %v", err)
	}
	if result.Status != task.ScheduleRunFailed || !strings.Contains(result.Detail, "task not found") {
		t.Fatalf("unexpected run result: %+v", result)
	}
	if rt.ListSchedules()[0].LastStatus != task.ScheduleRunFailed {
		t.Fatalf("run result not recorded: %+v", rt.ListSchedules()[0])
	}
}

func TestRunScheduleNowSkipsWhenAlreadyStopped(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{newDummyTask()})

	sch := newDummySchedule("dummy")
	sch.Action = task.ScheduleActionStop
	if err := rt.UpsertSchedule(sch); err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	result, err := rt.RunScheduleNow("sch-test")
	if err != nil {
		t.Fatalf("run schedule now: %v", err)
	}
	if result.Status != task.ScheduleRunSkipped || result.Detail != "already_stopped" {
		t.Fatalf("unexpected result: %+v", result)
	}

	got := rt.ListSchedules()[0]
	if got.LastStatus != task.ScheduleRunSkipped || got.LastRunAt == "" {
		t.Fatalf("run not recorded: %+v", got)
	}

	persisted, err := rt.TaskStore.LoadSchedules()
	if err != nil {
		t.Fatalf("load schedules: %v", err)
	}
	if persisted[0].LastStatus != task.ScheduleRunSkipped {
		t.Fatalf("run result not persisted: %+v", persisted[0])
	}
}

func TestStartSchedulerRegistersEnabledEntries(t *testing.T) {
	rt := newTestRuntime(t, []task.Config{newDummyTask()})
	t.Cleanup(func() { rt.Sched.Stop(time.Second) })

	if err := rt.UpsertSchedule(newDummySchedule("dummy")); err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	disabled := newDummySchedule("dummy")
	disabled.ID = "sch-off"
	disabled.Enabled = false
	if err := rt.UpsertSchedule(disabled); err != nil {
		t.Fatalf("create disabled schedule: %v", err)
	}

	if err := rt.StartScheduler(); err != nil {
		t.Fatalf("start scheduler: %v", err)
	}

	next, ok := rt.ScheduleNextRun("sch-test")
	if !ok || next.IsZero() {
		t.Fatalf("expected next run for enabled schedule")
	}
	if _, ok := rt.ScheduleNextRun("sch-off"); ok {
		t.Fatalf("disabled schedule should have no next run")
	}
}
