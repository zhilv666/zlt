package scheduler

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"zhulingtai/internal/process"
	"zhulingtai/internal/task"
)

// Controller is the slice of Runtime the scheduler drives tasks through.
// It must be backed by *app.Runtime — never by a cached *process.Manager,
// because task import replaces the Manager instance wholesale.
type Controller interface {
	State(taskID string) (process.RuntimeState, bool)
	Start(taskID string) error
	Stop(taskID string) error
	RestartTask(taskID string) error
}

// ResultFunc receives the outcome of every schedule execution (cron-fired or
// manual). Implementations must tolerate IDs that no longer exist: a job can
// finish after its schedule was deleted or reloaded.
type ResultFunc func(scheduleID string, at time.Time, status string, detail string)

type Scheduler struct {
	ctrl     Controller
	onResult ResultFunc

	mu      sync.Mutex
	cron    *cron.Cron
	entries map[string]cron.EntryID

	// inFlight guards each schedule against overlapping executions so a slow
	// run makes the next tick (or a manual run) skip instead of piling up.
	inFlightMu sync.Mutex
	inFlight   map[string]bool
}

func New(ctrl Controller, onResult ResultFunc) *Scheduler {
	return &Scheduler{
		ctrl:     ctrl,
		onResult: onResult,
		cron:     newCron(),
		entries:  map[string]cron.EntryID{},
		inFlight: map[string]bool{},
	}
}

func newCron() *cron.Cron {
	logger := cron.PrintfLogger(log.Default())
	return cron.New(cron.WithChain(cron.Recover(logger)))
}

// cronSpec renders the stored expression + timezone into a robfig/cron spec.
// Per-entry timezones use the standard parser's CRON_TZ prefix.
func cronSpec(sch task.Schedule) string {
	expr := strings.TrimSpace(sch.CronExpr)
	tz := strings.TrimSpace(sch.Timezone)
	if tz == "" {
		return expr
	}
	return "CRON_TZ=" + tz + " " + expr
}

// Validate checks the cron expression (5-field standard spec) and timezone.
func Validate(sch task.Schedule) error {
	expr := strings.TrimSpace(sch.CronExpr)
	if expr == "" {
		return errors.New("cron expression is required")
	}
	if tz := strings.TrimSpace(sch.Timezone); tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return fmt.Errorf("invalid timezone %q: %w", tz, err)
		}
	}
	if _, err := cron.ParseStandard(cronSpec(sch)); err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	switch sch.Action {
	case task.ScheduleActionStart, task.ScheduleActionStop, task.ScheduleActionRestart:
	default:
		return fmt.Errorf("invalid action %q", sch.Action)
	}
	return nil
}

// NextRun computes the next fire time of an expression from a reference time,
// independent of the running scheduler. Used for validation-time previews.
func NextRun(sch task.Schedule, from time.Time) (time.Time, error) {
	spec, err := cron.ParseStandard(cronSpec(sch))
	if err != nil {
		return time.Time{}, err
	}
	return spec.Next(from), nil
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cron.Start()
}

// Stop halts triggering and waits (bounded) for in-flight jobs to finish.
func (s *Scheduler) Stop(timeout time.Duration) {
	s.mu.Lock()
	ctx := s.cron.Stop()
	s.mu.Unlock()

	select {
	case <-ctx.Done():
	case <-time.After(timeout):
		slog.Warn("scheduler stop: timed out waiting for running jobs", "timeout", timeout)
	}
}

// Reload replaces all cron entries with the enabled schedules from the given
// list. Called on startup and after every schedule mutation.
func (s *Scheduler) Reload(schedules []task.Schedule) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, entryID := range s.entries {
		s.cron.Remove(entryID)
		delete(s.entries, id)
	}

	for _, sch := range schedules {
		if !sch.Enabled {
			continue
		}
		sch := sch
		entryID, err := s.cron.AddFunc(cronSpec(sch), func() {
			s.run(sch)
		})
		if err != nil {
			// Validation happens before persistence, so this is defensive.
			slog.Error("schedule: failed to register cron entry", "schedule", sch.ID, "err", err)
			continue
		}
		s.entries[sch.ID] = entryID
	}
}

// EntryNextRun reports the next fire time of a registered (enabled) schedule.
func (s *Scheduler) EntryNextRun(scheduleID string) (time.Time, bool) {
	s.mu.Lock()
	entryID, ok := s.entries[scheduleID]
	if !ok {
		s.mu.Unlock()
		return time.Time{}, false
	}
	entry := s.cron.Entry(entryID)
	s.mu.Unlock()

	if entry.ID == 0 || entry.Next.IsZero() {
		return time.Time{}, false
	}
	return entry.Next, true
}

// RunNow executes a schedule immediately, sharing the overlap guard and
// result recording with cron-fired runs.
func (s *Scheduler) RunNow(sch task.Schedule) task.ScheduleRunResult {
	return s.run(sch)
}

func (s *Scheduler) run(sch task.Schedule) task.ScheduleRunResult {
	if !s.tryAcquire(sch.ID) {
		result := task.ScheduleRunResult{Status: task.ScheduleRunSkipped, Detail: "previous run still in progress"}
		s.report(sch.ID, result)
		return result
	}
	defer s.release(sch.ID)

	result := s.execute(sch)
	s.report(sch.ID, result)
	return result
}

func (s *Scheduler) report(scheduleID string, result task.ScheduleRunResult) {
	slog.Info("schedule run", "schedule", scheduleID, "status", result.Status, "detail", result.Detail)
	if s.onResult != nil {
		s.onResult(scheduleID, time.Now(), result.Status, result.Detail)
	}
}

// execute maps the action onto the current task state.
//
// The manager's own semantics differ from what a scheduler wants: Start on a
// running task returns an error, Stop on a stopped task silently succeeds.
// Pre-checking the state lets both cases surface as explicit "skipped" runs.
func (s *Scheduler) execute(sch task.Schedule) task.ScheduleRunResult {
	state, ok := s.ctrl.State(sch.TaskID)
	if !ok {
		return task.ScheduleRunResult{Status: task.ScheduleRunFailed, Detail: "task not found: " + sch.TaskID}
	}
	running := isRunningStatus(state.Status)

	var err error
	switch sch.Action {
	case task.ScheduleActionStart:
		if running {
			return task.ScheduleRunResult{Status: task.ScheduleRunSkipped, Detail: "already_running"}
		}
		err = s.ctrl.Start(sch.TaskID)
	case task.ScheduleActionStop:
		if !running {
			return task.ScheduleRunResult{Status: task.ScheduleRunSkipped, Detail: "already_stopped"}
		}
		err = s.ctrl.Stop(sch.TaskID)
	case task.ScheduleActionRestart:
		err = s.ctrl.RestartTask(sch.TaskID)
	default:
		return task.ScheduleRunResult{Status: task.ScheduleRunFailed, Detail: "invalid action: " + string(sch.Action)}
	}

	if err != nil {
		return task.ScheduleRunResult{Status: task.ScheduleRunFailed, Detail: err.Error()}
	}
	return task.ScheduleRunResult{Status: task.ScheduleRunSuccess}
}

func isRunningStatus(status string) bool {
	return status == process.StatusRunning || status == process.StatusStarting || status == process.StatusStopping
}

func (s *Scheduler) tryAcquire(scheduleID string) bool {
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	if s.inFlight[scheduleID] {
		return false
	}
	s.inFlight[scheduleID] = true
	return true
}

func (s *Scheduler) release(scheduleID string) {
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	delete(s.inFlight, scheduleID)
}
