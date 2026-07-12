package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
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
	err := r.disableOrphanSchedulesLocked()
	schedules := r.copySchedulesLocked()
	r.mu.Unlock()

	r.Sched.Start()
	r.Sched.Reload(schedules)
	return err
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

	now := nowRFC3339()
	if i := r.findScheduleIndexLocked(sch.ID); i >= 0 {
		// Run history and creation time are server-owned; edits never reset them.
		prev := r.Schedules[i]
		sch.CreatedAt = prev.CreatedAt
		sch.LastRunAt = prev.LastRunAt
		sch.LastStatus = prev.LastStatus
		sch.LastDetail = prev.LastDetail
		sch.UpdatedAt = now
		r.Schedules[i] = sch
	} else {
		sch.CreatedAt = now
		sch.UpdatedAt = now
		sch.LastRunAt = ""
		sch.LastStatus = ""
		sch.LastDetail = ""
		r.Schedules = append(r.Schedules, sch)
	}

	if err := r.TaskStore.SaveSchedules(r.Schedules); err != nil {
		return err
	}
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
	r.Schedules = append(r.Schedules[:i], r.Schedules[i+1:]...)

	if err := r.TaskStore.SaveSchedules(r.Schedules); err != nil {
		return err
	}
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
	r.Schedules[i].Enabled = enabled
	r.Schedules[i].UpdatedAt = nowRFC3339()

	if err := r.TaskStore.SaveSchedules(r.Schedules); err != nil {
		return err
	}
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
	r.Schedules[i].LastRunAt = at.Format(time.RFC3339)
	r.Schedules[i].LastStatus = status
	r.Schedules[i].LastDetail = detail

	if err := r.TaskStore.SaveSchedules(r.Schedules); err != nil {
		log.Printf("schedule %s: failed to persist run result: %v", scheduleID, err)
	}
}

// disableOrphanSchedulesLocked disables enabled schedules whose task no longer
// exists (e.g. removed by an import) and records why. Caller holds r.mu.
func (r *Runtime) disableOrphanSchedulesLocked() error {
	changed := false
	now := nowRFC3339()
	for i := range r.Schedules {
		if !r.Schedules[i].Enabled || r.taskExistsLocked(r.Schedules[i].TaskID) {
			continue
		}
		r.Schedules[i].Enabled = false
		r.Schedules[i].LastDetail = "task not found: " + r.Schedules[i].TaskID + " (schedule disabled)"
		r.Schedules[i].UpdatedAt = now
		changed = true
		log.Printf("schedule %s: task %s missing, disabled", r.Schedules[i].ID, r.Schedules[i].TaskID)
	}
	if !changed {
		return nil
	}
	return r.TaskStore.SaveSchedules(r.Schedules)
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
