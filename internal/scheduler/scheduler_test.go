package scheduler

import (
	"errors"
	"sync"
	"testing"
	"time"

	"zhulingtai/internal/process"
	"zhulingtai/internal/task"
)

type fakeController struct {
	mu           sync.Mutex
	states       map[string]string
	startCalls   []string
	stopCalls    []string
	restartCalls []string
	startErr     error
	stopErr      error
	restartErr   error
}

func (f *fakeController) State(taskID string) (process.RuntimeState, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	status, ok := f.states[taskID]
	return process.RuntimeState{TaskID: taskID, Status: status}, ok
}

func (f *fakeController) Start(taskID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls = append(f.startCalls, taskID)
	return f.startErr
}

func (f *fakeController) Stop(taskID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls = append(f.stopCalls, taskID)
	return f.stopErr
}

func (f *fakeController) RestartTask(taskID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restartCalls = append(f.restartCalls, taskID)
	return f.restartErr
}

func schedule(action task.ScheduleAction) task.Schedule {
	return task.Schedule{
		ID:       "sch-test",
		TaskID:   "demo",
		Name:     "test",
		CronExpr: "*/5 * * * *",
		Action:   action,
		Enabled:  true,
	}
}

func TestValidate(t *testing.T) {
	valid := []task.Schedule{
		{CronExpr: "*/5 * * * *", Action: task.ScheduleActionStart},
		{CronExpr: "0 8 * * 1-5", Action: task.ScheduleActionStop},
		{CronExpr: "0 23 * * *", Action: task.ScheduleActionRestart},
		{CronExpr: "0 8 * * 1-5", Timezone: "Asia/Shanghai", Action: task.ScheduleActionStart},
		{CronExpr: "30 6 1 * *", Timezone: "UTC", Action: task.ScheduleActionStart},
	}
	for _, sch := range valid {
		if err := Validate(sch); err != nil {
			t.Fatalf("expected valid %q tz=%q: %v", sch.CronExpr, sch.Timezone, err)
		}
	}

	invalid := []task.Schedule{
		{CronExpr: "", Action: task.ScheduleActionStart},
		{CronExpr: "not a cron", Action: task.ScheduleActionStart},
		{CronExpr: "61 * * * *", Action: task.ScheduleActionStart},
		{CronExpr: "* * * *", Action: task.ScheduleActionStart},
		{CronExpr: "*/5 * * * *", Timezone: "Mars/Olympus", Action: task.ScheduleActionStart},
		{CronExpr: "*/5 * * * *", Action: "pause"},
	}
	for _, sch := range invalid {
		if err := Validate(sch); err == nil {
			t.Fatalf("expected invalid %q tz=%q action=%q", sch.CronExpr, sch.Timezone, sch.Action)
		}
	}
}

func TestNextRunWithTimezone(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	sch := task.Schedule{CronExpr: "0 8 * * 1-5", Timezone: "Asia/Shanghai", Action: task.ScheduleActionStart}
	// Friday noon Shanghai time → next weekday 08:00 is Monday.
	from := time.Date(2026, 7, 10, 12, 0, 0, 0, shanghai)
	next, err := NextRun(sch, from)
	if err != nil {
		t.Fatalf("next run: %v", err)
	}
	want := time.Date(2026, 7, 13, 8, 0, 0, 0, shanghai)
	if !next.Equal(want) {
		t.Fatalf("next run = %s, want %s", next, want)
	}
}

func TestExecuteStateMapping(t *testing.T) {
	cases := []struct {
		name       string
		action     task.ScheduleAction
		taskStatus string
		taskExists bool
		wantStatus string
		wantDetail string
		wantCalls  func(f *fakeController) int
	}{
		{"start on stopped starts", task.ScheduleActionStart, process.StatusStopped, true, task.ScheduleRunSuccess, "", func(f *fakeController) int { return len(f.startCalls) }},
		{"start on running skips", task.ScheduleActionStart, process.StatusRunning, true, task.ScheduleRunSkipped, "already_running", func(f *fakeController) int { return 1 - len(f.startCalls) }},
		{"start on starting skips", task.ScheduleActionStart, process.StatusStarting, true, task.ScheduleRunSkipped, "already_running", func(f *fakeController) int { return 1 - len(f.startCalls) }},
		{"stop on running stops", task.ScheduleActionStop, process.StatusRunning, true, task.ScheduleRunSuccess, "", func(f *fakeController) int { return len(f.stopCalls) }},
		{"stop on stopped skips", task.ScheduleActionStop, process.StatusStopped, true, task.ScheduleRunSkipped, "already_stopped", func(f *fakeController) int { return 1 - len(f.stopCalls) }},
		{"stop on exited skips", task.ScheduleActionStop, process.StatusExited, true, task.ScheduleRunSkipped, "already_stopped", func(f *fakeController) int { return 1 - len(f.stopCalls) }},
		{"restart on running restarts", task.ScheduleActionRestart, process.StatusRunning, true, task.ScheduleRunSuccess, "", func(f *fakeController) int { return len(f.restartCalls) }},
		{"restart on stopped restarts", task.ScheduleActionRestart, process.StatusStopped, true, task.ScheduleRunSuccess, "", func(f *fakeController) int { return len(f.restartCalls) }},
		{"missing task fails", task.ScheduleActionStart, "", false, task.ScheduleRunFailed, "task not found: demo", func(f *fakeController) int { return 1 - len(f.startCalls) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := &fakeController{states: map[string]string{}}
			if tc.taskExists {
				ctrl.states["demo"] = tc.taskStatus
			}
			s := New(ctrl, nil)

			result := s.RunNow(schedule(tc.action))
			if result.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (detail=%q)", result.Status, tc.wantStatus, result.Detail)
			}
			if result.Detail != tc.wantDetail {
				t.Fatalf("detail = %q, want %q", result.Detail, tc.wantDetail)
			}
			if got := tc.wantCalls(ctrl); got != 1 {
				t.Fatalf("unexpected controller calls: %+v", ctrl)
			}
		})
	}
}

func TestExecuteControllerErrorBecomesFailed(t *testing.T) {
	ctrl := &fakeController{
		states:   map[string]string{"demo": process.StatusStopped},
		startErr: errors.New("boom"),
	}
	s := New(ctrl, nil)

	result := s.RunNow(schedule(task.ScheduleActionStart))
	if result.Status != task.ScheduleRunFailed || result.Detail != "boom" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestOverlapSkip(t *testing.T) {
	ctrl := &fakeController{states: map[string]string{"demo": process.StatusStopped}}

	var results []task.ScheduleRunResult
	s := New(ctrl, func(id string, at time.Time, status, detail string) {
		results = append(results, task.ScheduleRunResult{Status: status, Detail: detail})
	})

	sch := schedule(task.ScheduleActionStart)
	if !s.tryAcquire(sch.ID) {
		t.Fatalf("initial acquire failed")
	}

	result := s.RunNow(sch)
	if result.Status != task.ScheduleRunSkipped || result.Detail != "previous run still in progress" {
		t.Fatalf("expected overlap skip, got %+v", result)
	}
	if len(ctrl.startCalls) != 0 {
		t.Fatalf("controller should not be called while in flight")
	}

	s.release(sch.ID)
	result = s.RunNow(sch)
	if result.Status != task.ScheduleRunSuccess {
		t.Fatalf("expected success after release, got %+v", result)
	}

	if len(results) != 2 || results[0].Status != task.ScheduleRunSkipped || results[1].Status != task.ScheduleRunSuccess {
		t.Fatalf("onResult did not record both runs: %+v", results)
	}
}

func TestReloadAndEntryNextRun(t *testing.T) {
	ctrl := &fakeController{states: map[string]string{"demo": process.StatusStopped}}
	s := New(ctrl, nil)
	s.Start()
	defer s.Stop(time.Second)

	enabled := schedule(task.ScheduleActionStart)
	disabled := schedule(task.ScheduleActionStop)
	disabled.ID = "sch-disabled"
	disabled.Enabled = false

	s.Reload([]task.Schedule{enabled, disabled})

	next, ok := s.EntryNextRun(enabled.ID)
	if !ok || next.IsZero() || !next.After(time.Now().Add(-time.Second)) {
		t.Fatalf("expected next run for enabled schedule, got %v ok=%v", next, ok)
	}
	if _, ok := s.EntryNextRun(disabled.ID); ok {
		t.Fatalf("disabled schedule should not be registered")
	}

	s.Reload(nil)
	if _, ok := s.EntryNextRun(enabled.ID); ok {
		t.Fatalf("entry should be gone after reload with empty list")
	}
}
