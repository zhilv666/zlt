package app

import (
	"zhulingtai/internal/api"
	"zhulingtai/internal/task"
)

// ReorderTasks persists a new task order. ids is the complete target order;
// baseIDs is the order before the drag, used for optimistic-concurrency
// detection. The method:
//  1. Validates ids is a complete, unique permutation of current tasks (400).
//  2. Validates baseIDs matches the current in-memory order exactly (409).
//  3. Persists only the sort_order column (no DELETE/re-insert, no process
//     manager rebuild, no cron re-registration).
//  4. Replaces the in-memory slice only after the transaction commits.
func (r *Runtime) ReorderTasks(ids, baseIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := validateReorder(ids, baseIDs, taskList(r.Tasks)); err != nil {
		return err
	}

	if err := r.TaskStore.ReorderTasks(ids); err != nil {
		return err
	}

	// Build the new in-memory order from the validated id list.
	byID := make(map[string]int, len(r.Tasks))
	for i, cfg := range r.Tasks {
		byID[cfg.ID] = i
	}
	reordered := make([]task.Config, len(ids))
	for i, id := range ids {
		reordered[i] = r.Tasks[byID[id]]
	}
	r.Tasks = reordered
	return nil
}

// ReorderSchedules is the schedule counterpart of ReorderTasks.
func (r *Runtime) ReorderSchedules(ids, baseIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := validateReorder(ids, baseIDs, scheduleList(r.Schedules)); err != nil {
		return err
	}

	if err := r.TaskStore.ReorderSchedules(ids); err != nil {
		return err
	}

	byID := make(map[string]int, len(r.Schedules))
	for i, sch := range r.Schedules {
		byID[sch.ID] = i
	}
	reordered := make([]task.Schedule, len(ids))
	for i, id := range ids {
		reordered[i] = r.Schedules[byID[id]]
	}
	r.Schedules = reordered
	return nil
}

// validateReorder checks that ids is a complete, unique permutation of the
// current items, and that baseIDs matches the current order. It works for
// both tasks and schedules via the idList interface.
func validateReorder(ids, baseIDs []string, current idList) error {
	if len(ids) != current.Len() {
		return api.ErrReorderInvalid
	}

	// Uniqueness + existence: every id must appear exactly once.
	seen := make(map[string]struct{}, len(ids))
	currentIDs := current.IDs()
	currentSet := make(map[string]struct{}, len(currentIDs))
	for _, id := range currentIDs {
		currentSet[id] = struct{}{}
	}
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			return api.ErrReorderInvalid
		}
		if _, exists := currentSet[id]; !exists {
			return api.ErrReorderInvalid
		}
		seen[id] = struct{}{}
	}

	// Optimistic concurrency: baseIDs must match the current order exactly.
	if len(baseIDs) != len(currentIDs) {
		return api.ErrReorderConflict
	}
	for i, id := range baseIDs {
		if id != currentIDs[i] {
			return api.ErrReorderConflict
		}
	}
	return nil
}

// idList is a minimal interface so validateReorder works for both tasks and
// schedules without duplicating logic.
type idList interface {
	Len() int
	IDs() []string
}

// taskList adapts []task.Config to idList.
type taskList []task.Config

func (t taskList) Len() int      { return len(t) }
func (t taskList) IDs() []string { return taskIDs(t) }

// scheduleList adapts []task.Schedule to idList.
type scheduleList []task.Schedule

func (s scheduleList) Len() int      { return len(s) }
func (s scheduleList) IDs() []string { return scheduleIDs(s) }

func taskIDs(tasks []task.Config) []string {
	out := make([]string, len(tasks))
	for i, cfg := range tasks {
		out[i] = cfg.ID
	}
	return out
}

func scheduleIDs(schedules []task.Schedule) []string {
	out := make([]string, len(schedules))
	for i, sch := range schedules {
		out[i] = sch.ID
	}
	return out
}

// currentTaskIDs returns the IDs of the in-memory task list under the read
// lock, for the API handler to send back as base_ids context.
func (r *Runtime) currentTaskIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return taskIDs(r.Tasks)
}

func (r *Runtime) currentScheduleIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return scheduleIDs(r.Schedules)
}
