package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"zhulingtai/internal/scheduler"
	"zhulingtai/internal/task"
)

// scheduleStopTimeout bounds how long shutdown waits for in-flight schedule
// executions; a restart job can legitimately take stop-timeout + start time.
const scheduleStopTimeout = 10 * time.Second

// StartScheduler begins cron triggering. Call after the runtime is fully
// constructed; safe to call once during application startup.
func (r *Runtime) StartScheduler() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.disableOrphanSchedulesLocked(); err != nil {
		return err
	}
	r.Sched.Start()
	r.reloadSchedulerLocked()
	return nil
}

func (r *Runtime) ListSchedules() []task.Schedule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.copySchedulesLocked()
}

func (r *Runtime) UpsertSchedule(sch task.Schedule) error {
	sch = normalizeSchedule(sch)
	if err := validateSchedule(sch); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.taskExistsLocked(sch.TaskID) {
		return fmt.Errorf("task %q not found", sch.TaskID)
	}

	candidate := r.copySchedulesLocked()
	now := nowRFC3339()
	if i := r.findScheduleIndexLocked(sch.ID); i >= 0 {
		// Run history and creation time are server-owned; edits never reset them.
		prev := r.Schedules[i]
		sch.CreatedAt = prev.CreatedAt
		sch.LastRunAt = prev.LastRunAt
		sch.LastStatus = prev.LastStatus
		sch.LastDetail = prev.LastDetail
		sch.UpdatedAt = now
		candidate[i] = sch
	} else {
		sch.CreatedAt = now
		sch.UpdatedAt = now
		sch.LastRunAt = ""
		sch.LastStatus = ""
		sch.LastDetail = ""
		candidate = append(candidate, sch)
	}

	if err := r.TaskStore.SaveSchedules(candidate); err != nil {
		return err
	}
	r.Schedules = candidate
	r.reloadSchedulerLocked()
	return nil
}

func (r *Runtime) DeleteSchedule(scheduleID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	i := r.findScheduleIndexLocked(scheduleID)
	if i < 0 {
		return fmt.Errorf("schedule %q not found", scheduleID)
	}
	candidate := r.copySchedulesLocked()
	candidate = append(candidate[:i], candidate[i+1:]...)

	if err := r.TaskStore.SaveSchedules(candidate); err != nil {
		return err
	}
	r.Schedules = candidate
	r.reloadSchedulerLocked()
	return nil
}

func (r *Runtime) SetScheduleEnabled(scheduleID string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	i := r.findScheduleIndexLocked(scheduleID)
	if i < 0 {
		return fmt.Errorf("schedule %q not found", scheduleID)
	}
	if enabled && !r.taskExistsLocked(r.Schedules[i].TaskID) {
		return fmt.Errorf("task %q not found, schedule stays disabled", r.Schedules[i].TaskID)
	}
	if r.Schedules[i].Enabled == enabled {
		return nil
	}
	candidate := r.copySchedulesLocked()
	candidate[i].Enabled = enabled
	candidate[i].UpdatedAt = nowRFC3339()

	if err := r.TaskStore.SaveSchedules(candidate); err != nil {
		return err
	}
	r.Schedules = candidate
	r.reloadSchedulerLocked()
	return nil
}

// RunScheduleNow executes a schedule immediately (even a disabled one, so a
// plan can be tested before enabling). The overlap guard and result recording
// are shared with cron-fired runs.
func (r *Runtime) RunScheduleNow(scheduleID string) (task.ScheduleRunResult, error) {
	r.mu.RLock()
	i := r.findScheduleIndexLocked(scheduleID)
	if i < 0 {
		r.mu.RUnlock()
		return task.ScheduleRunResult{}, fmt.Errorf("schedule %q not found", scheduleID)
	}
	sch := r.Schedules[i]
	r.mu.RUnlock()

	// Must not hold r.mu here: the run reports back via recordScheduleResult,
	// which takes the write lock.
	return r.Sched.RunNow(sch), nil
}

func (r *Runtime) ScheduleNextRun(scheduleID string) (time.Time, bool) {
	return r.Sched.EntryNextRun(scheduleID)
}

// recordScheduleResult is the scheduler's ResultFunc. Runs can complete after
// their schedule was deleted or replaced; unknown IDs are ignored.
func (r *Runtime) recordScheduleResult(scheduleID string, at time.Time, status string, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	i := r.findScheduleIndexLocked(scheduleID)
	if i < 0 {
		return
	}
	candidate := r.copySchedulesLocked()
	candidate[i].LastRunAt = at.Format(time.RFC3339)
	candidate[i].LastStatus = status
	candidate[i].LastDetail = detail

	if err := r.TaskStore.SaveSchedules(candidate); err != nil {
		slog.Error("schedule: failed to persist run result", "schedule", scheduleID, "err", err)
		return
	}
	r.Schedules = candidate
}

// disableOrphanSchedulesLocked disables enabled schedules whose task no longer
// exists (e.g. removed by an import) and records why. Caller holds r.mu.
func (r *Runtime) disableOrphanSchedulesLocked() error {
	candidate, changed := disableOrphanSchedules(r.Tasks, r.copySchedulesLocked())
	if !changed {
		return nil
	}
	if err := r.TaskStore.SaveSchedules(candidate); err != nil {
		return err
	}
	r.Schedules = candidate
	return nil
}

func disableOrphanSchedules(tasks []task.Config, schedules []task.Schedule) ([]task.Schedule, bool) {
	ids := make(map[string]bool, len(tasks))
	for _, cfg := range tasks {
		ids[cfg.ID] = true
	}
	changed := false
	now := nowRFC3339()
	for i := range schedules {
		if !schedules[i].Enabled || ids[schedules[i].TaskID] {
			continue
		}
		schedules[i].Enabled = false
		schedules[i].LastDetail = "task not found: " + schedules[i].TaskID + " (schedule disabled)"
		schedules[i].UpdatedAt = now
		changed = true
	}
	return schedules, changed
}

func (r *Runtime) reloadSchedulerLocked() {
	if r.Sched != nil {
		r.Sched.Reload(r.copySchedulesLocked())
	}
}

func (r *Runtime) copySchedulesLocked() []task.Schedule {
	out := make([]task.Schedule, len(r.Schedules))
	copy(out, r.Schedules)
	return out
}

func (r *Runtime) findScheduleIndexLocked(scheduleID string) int {
	for i := range r.Schedules {
		if r.Schedules[i].ID == scheduleID {
			return i
		}
	}
	return -1
}

func (r *Runtime) taskExistsLocked(taskID string) bool {
	for i := range r.Tasks {
		if r.Tasks[i].ID == taskID {
			return true
		}
	}
	return false
}

func (r *Runtime) countSchedulesForTaskLocked(taskID string) int {
	n := 0
	for i := range r.Schedules {
		if r.Schedules[i].TaskID == taskID {
			n++
		}
	}
	return n
}

func normalizeSchedule(sch task.Schedule) task.Schedule {
	sch.ID = strings.TrimSpace(sch.ID)
	sch.TaskID = strings.TrimSpace(sch.TaskID)
	sch.Name = strings.TrimSpace(sch.Name)
	sch.CronExpr = strings.TrimSpace(sch.CronExpr)
	sch.Timezone = strings.TrimSpace(sch.Timezone)
	if sch.ID == "" {
		sch.ID = generateScheduleID()
	}
	if sch.Name == "" {
		sch.Name = sch.ID
	}
	return sch
}

func validateSchedule(sch task.Schedule) error {
	if sch.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	return scheduler.Validate(sch)
}

func generateScheduleID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("sch-%d", time.Now().UnixNano())
	}
	return "sch-" + hex.EncodeToString(buf)
}

func nowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}
