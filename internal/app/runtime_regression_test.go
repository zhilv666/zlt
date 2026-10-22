package app

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"zhulingtai/internal/process"
	"zhulingtai/internal/task"
)

func TestRegressionTaskSaveFailureKeepsRuntime(t *testing.T) {
	cfg := newDummyTask()
	rt := newTestRuntime(t, []task.Config{cfg})
	if err := rt.TaskStore.Close(); err != nil {
		t.Fatal(err)
	}
	changed := cfg
	changed.Name = "Unpersisted name"
	if err := rt.UpsertTask(changed); err == nil {
		t.Fatal("expected persistence failure")
	}
	if got := rt.ListTasks()[0].Name; got != cfg.Name {
		t.Fatalf("failed save changed active in-memory task from %q to %q", cfg.Name, got)
	}
	if got := rt.Manager.Tasks()[0].Name; got != cfg.Name {
		t.Fatalf("manager changed after failed save: %s", got)
	}
}

func TestRegressionScheduleDeleteFailureKeepsRuntime(t *testing.T) {
	cfg := newDummyTask()
	rt := newTestRuntime(t, []task.Config{cfg})
	sch := task.Schedule{ID: "regression-plan", Name: "Review", TaskID: cfg.ID, CronExpr: "* * * * *", Action: task.ScheduleActionStart, Enabled: true}
	if err := rt.UpsertSchedule(sch); err != nil {
		t.Fatal(err)
	}
	if err := rt.TaskStore.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rt.DeleteSchedule(sch.ID); err == nil {
		t.Fatal("expected persistence failure")
	}
	if got := len(rt.ListSchedules()); got != 1 {
		t.Fatalf("failed delete removed schedule from memory while its persisted configuration and cron registration remained (count=%d)", got)
	}
}

func TestRegressionImportManagerRace(t *testing.T) {
	rt := newTestRuntime(t, nil)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = rt.State("missing")
				_ = rt.States()
				_ = rt.Start("missing")
				_ = rt.Stop("missing")
				_ = rt.RestartTask("missing")
				_ = rt.ClearLogs("missing")
			}
		}
	}()
	defer func() { close(stop); <-done }()
	for i := 0; i < 20; i++ {
		if err := rt.ReplaceTasks(nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRegressionTaskPersistenceFailures(t *testing.T) {
	for _, action := range []string{"create", "delete", "policy", "import"} {
		t.Run(action, func(t *testing.T) {
			cfg := newDummyTask()
			rt := newTestRuntime(t, []task.Config{cfg})
			before := rt.ListTasks()
			managerBefore := rt.Manager.Tasks()
			_ = rt.TaskStore.Close()
			var err error
			switch action {
			case "create":
				cfg.ID = "new"
				err = rt.UpsertTask(cfg)
			case "delete":
				err = rt.DeleteTask(cfg.ID)
			case "policy":
				err = rt.SetRestartOnCrash(cfg.ID, true)
			case "import":
				err = rt.ReplaceTasks(nil)
			}
			if err == nil {
				t.Fatal("expected persistence failure")
			}
			if !reflect.DeepEqual(before, rt.ListTasks()) || !reflect.DeepEqual(managerBefore, rt.Manager.Tasks()) {
				t.Fatal("failed save changed active tasks")
			}
		})
	}
}

func TestRegressionSchedulePersistenceFailures(t *testing.T) {
	for _, action := range []string{"create", "edit", "disable", "delete", "import"} {
		t.Run(action, func(t *testing.T) {
			rt := newTestRuntime(t, []task.Config{newDummyTask()})
			sch := newDummySchedule("dummy")
			if err := rt.UpsertSchedule(sch); err != nil {
				t.Fatal(err)
			}
			if err := rt.StartScheduler(); err != nil {
				t.Fatal(err)
			}
			defer rt.Sched.Stop(time.Second)
			before := rt.ListSchedules()
			beforeTasks := rt.ListTasks()
			next, ok := rt.ScheduleNextRun(sch.ID)
			if !ok {
				t.Fatal("missing cron entry")
			}
			_ = rt.TaskStore.Close()
			var err error
			switch action {
			case "create":
				sch.ID = "new"
				err = rt.UpsertSchedule(sch)
			case "edit":
				sch.CronExpr = "*/5 * * * *"
				err = rt.UpsertSchedule(sch)
			case "disable":
				err = rt.SetScheduleEnabled(sch.ID, false)
			case "delete":
				err = rt.DeleteSchedule(sch.ID)
			case "import":
				err = rt.ReplaceTasks(nil)
			}
			if err == nil {
				t.Fatal("expected persistence failure")
			}
			if !reflect.DeepEqual(before, rt.ListSchedules()) || !reflect.DeepEqual(beforeTasks, rt.ListTasks()) {
				t.Fatal("failed save changed active configuration")
			}
			afterNext, ok := rt.ScheduleNextRun("sch-test")
			if !ok || !afterNext.Equal(next) {
				t.Fatal("failed save changed cron registration")
			}
		})
	}
}

func TestRegressionRuntimeHelper(t *testing.T) {
	if os.Getenv("ZLT_RUNTIME_HELPER") != "1" {
		return
	}
	if marker := os.Getenv("ZLT_RUNTIME_MARKER"); marker != "" {
		file, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			os.Exit(9)
		}
		_, _ = file.WriteString("start\n")
		_ = file.Close()
	}
	if os.Getenv("ZLT_RUNTIME_CRASH") == "1" {
		os.Exit(7)
	}
	for {
		time.Sleep(time.Second)
	}
}

func runtimeHelperTask(t *testing.T) task.Config {
	return task.Config{ID: "helper", Name: "Helper", Program: mustExecutable(t), WorkDir: t.TempDir(), Args: []string{"-test.run=^TestRegressionRuntimeHelper$"}, Env: []string{"ZLT_RUNTIME_HELPER=1"}, StopTimeoutSec: 1}
}

func TestRegressionImportAndStartStayManaged(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := runtimeHelperTask(t)
	rt := newTestRuntime(t, []task.Config{cfg})
	defer rt.StopAllTasks()
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; results <- rt.Start(cfg.ID) }()
	go func() { <-start; results <- rt.ReplaceTasks(nil) }()
	close(start)
	e1, e2 := <-results, <-results
	if e1 == nil && e2 == nil {
		t.Fatal("import and start both succeeded, running task lost its controller")
	}
	for _, state := range rt.States() {
		if state.Status == process.StatusRunning {
			if err := rt.Stop(state.TaskID); err != nil {
				t.Fatal(err)
			}
			state, _ := rt.State(state.TaskID)
			if state.PID != 0 || state.Status != process.StatusStopped {
				t.Fatalf("task did not stop: %+v", state)
			}
		}
	}
}

func TestRegressionRuntimeCancelsPendingRestarts(t *testing.T) {
	for _, action := range []string{"shutdown", "import"} {
		t.Run(action, func(t *testing.T) {
			t.Chdir(t.TempDir())
			cfg := runtimeHelperTask(t)
			marker := filepath.Join(t.TempDir(), "starts.txt")
			cfg.Env = append(cfg.Env, "ZLT_RUNTIME_CRASH=1", "ZLT_RUNTIME_MARKER="+marker)
			cfg.RestartOnCrash = true
			cfg.RestartDelaySec = 1
			cfg.MaxRestartCount = 1
			rt := newTestRuntime(t, []task.Config{cfg})
			defer rt.StopAllTasks()
			if err := rt.Start(cfg.ID); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				state, _ := rt.State(cfg.ID)
				if state.Status == process.StatusExited {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("did not crash: %+v", state)
				}
				time.Sleep(10 * time.Millisecond)
			}
			if action == "import" {
				if err := rt.ReplaceTasks(nil); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := rt.StopAllTasks(); err != nil {
					t.Fatal(err)
				}
			}
			time.Sleep(3 * time.Second)
			data, err := os.ReadFile(marker)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Count(data, []byte("start\n")) != 1 {
				t.Fatalf("task restarted after %s", action)
			}
		})
	}
}
