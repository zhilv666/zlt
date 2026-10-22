package process

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"zhulingtai/internal/task"
)

func TestRegressionHelperProcess(t *testing.T) {
	if os.Getenv("ZLT_REGRESSION_HELPER") != "1" {
		return
	}
	if marker := os.Getenv("ZLT_REGRESSION_MARKER"); marker != "" {
		f, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			os.Exit(9)
		}
		_, _ = f.WriteString("start\n")
		_ = f.Close()
	}
	if os.Getenv("ZLT_REGRESSION_MODE") == "crash" {
		os.Exit(7)
	}
	for {
		time.Sleep(time.Second)
	}
}

func regressionExecutable(t *testing.T) string {
	t.Helper()
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("zlt-regression-child-%d", time.Now().UnixNano())
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dst := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return dst
}

func regressionTask(t *testing.T, mode string) task.Config {
	t.Helper()
	return task.Config{
		ID: "regression", Name: "Review", Program: regressionExecutable(t),
		WorkDir: t.TempDir(), Args: []string{"-test.run=^TestRegressionHelperProcess$"},
		Env:            []string{"ZLT_REGRESSION_HELPER=1", "ZLT_REGRESSION_MODE=" + mode},
		StopTimeoutSec: 1,
	}
}

func regressionWaitState(t *testing.T, m *Manager, id, status string) RuntimeState {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		if state, ok := m.State(id); ok && state.Status == status {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	state, _ := m.State(id)
	t.Fatalf("did not reach %s, state=%+v", status, state)
	return state
}

func TestRegressionWindowsExistingPID(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows scanner")
	}
	cfg := regressionTask(t, "hold")
	cmd := exec.Command(cfg.Program, cfg.Args...)
	cmd.Dir = cfg.WorkDir
	cmd.Env = append(os.Environ(), cfg.Env...)
	prepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	manager := NewManager([]task.Config{cfg})
	state, _ := manager.State(cfg.ID)
	if state.Status != StatusRunning || state.PID != cmd.Process.Pid {
		t.Fatalf("existing child PID=%d, adopted state=%+v", cmd.Process.Pid, state)
	}
}

func TestRegressionCancelPendingRestart(t *testing.T) {
	for _, action := range []string{"stop", "disable_policy"} {
		t.Run(action, func(t *testing.T) {
			t.Chdir(t.TempDir())
			cfg := regressionTask(t, "crash")
			marker := filepath.Join(t.TempDir(), "starts.txt")
			cfg.Env = append(cfg.Env, "ZLT_REGRESSION_MARKER="+marker)
			cfg.RestartOnCrash = true
			cfg.RestartDelaySec = 1
			cfg.MaxRestartCount = 1
			m := NewManager(nil)
			m.UpsertTask(cfg)
			if err := m.Start(cfg.ID); err != nil {
				t.Fatal(err)
			}
			regressionWaitState(t, m, cfg.ID, StatusExited)
			if action == "stop" {
				if err := m.Stop(cfg.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				cfg.RestartOnCrash = false
				m.UpsertTask(cfg)
			}
			time.Sleep(2500 * time.Millisecond)
			data, err := os.ReadFile(marker)
			if err != nil {
				t.Fatal(err)
			}
			if starts := bytes.Count(data, []byte("start\n")); starts != 1 {
				t.Fatalf("after %s during restart delay: executable started %d times", action, starts)
			}
		})
	}
}

func TestRegressionRelativeProgram(t *testing.T) {
	for _, workdir := range []string{".", "worker"} {
		t.Run(workdir, func(t *testing.T) {
			t.Chdir(t.TempDir())
			cfg := regressionTask(t, "crash")
			data, err := os.ReadFile(cfg.Program)
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.Base(cfg.Program)
			if err := os.MkdirAll(workdir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workdir, name), data, 0o755); err != nil {
				t.Fatal(err)
			}
			cfg.Program = "./" + name
			cfg.WorkDir = workdir
			m := NewManager(nil)
			m.UpsertTask(cfg)
			if err := m.Start(cfg.ID); err != nil {
				t.Fatalf("existing executable with program=%q workdir=%q did not start: %v", cfg.Program, workdir, err)
			}
			regressionWaitState(t, m, cfg.ID, StatusExited)
		})
	}
}

func TestRegressionDifferentAbsoluteScript(t *testing.T) {
	if commandMatchesTask("/usr/bin/python3 /srv/service-a/main.py", "/usr/bin/python3", []string{"/srv/service-b/main.py"}) {
		t.Fatal("task for service-b incorrectly adopted service-a with a different absolute script path")
	}
}

func TestRegressionExactCommandIdentity(t *testing.T) {
	for _, tc := range []struct {
		line, program string
		args          []string
	}{
		{`/srv/service-a/main server`, `/srv/service-b/main`, []string{"server"}},
		{`/usr/bin/python3 /srv/a/main.py`, `/usr/bin/python3`, []string{"/srv/b/main.py"}},
		{`/usr/bin/python3 a/main.py`, `/usr/bin/python3`, []string{"b/main.py"}},
		{`/usr/bin/python3 main.py TOKEN`, `/usr/bin/python3`, []string{"main.py", "token"}},
		{`/usr/bin/python3 main.py --other`, `/usr/bin/python3`, []string{"main.py"}},
		{`/usr/bin/python3 /srv/app`, `/srv/app`, nil},
	} {
		if commandMatchesTask(tc.line, tc.program, tc.args) {
			t.Fatalf("incorrectly matched %q to %q %v", tc.line, tc.program, tc.args)
		}
	}
}

func TestRegressionAutoRestartStillRuns(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := regressionTask(t, "crash")
	marker := filepath.Join(t.TempDir(), "starts.txt")
	cfg.Env = append(cfg.Env, "ZLT_REGRESSION_MARKER="+marker)
	cfg.RestartOnCrash = true
	cfg.RestartDelaySec = 1
	cfg.MaxRestartCount = 1
	m := NewManager(nil)
	m.UpsertTask(cfg)
	defer m.Stop(cfg.ID)
	if err := m.Start(cfg.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(marker)
		state, _ := m.State(cfg.ID)
		if bytes.Count(data, []byte("start\n")) == 2 && state.Status == StatusExited {
			m.mu.RLock()
			pending := m.procs[cfg.ID].restartCancel != nil
			m.mu.RUnlock()
			if pending {
				t.Fatal("restart limit did not prevent another retry")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("automatic restart did not run")
}

func TestRegressionAdoptionPreservesArgumentBoundaries(t *testing.T) {
	cfg := regressionTask(t, "hold")
	cfg.Args = append(cfg.Args, "--", "path with spaces", "", `quoted"value`, `C:\trailing\`)
	cmd := exec.Command(cfg.Program, cfg.Args...)
	cmd.Dir = cfg.WorkDir
	cmd.Env = append(os.Environ(), cfg.Env...)
	prepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	m := NewManager([]task.Config{cfg})
	state, _ := m.State(cfg.ID)
	if state.PID != cmd.Process.Pid {
		t.Fatalf("argv boundaries lost during adoption: %+v", state)
	}
	if !reflect.DeepEqual(m.Tasks()[0].Args, cfg.Args) {
		t.Fatal("task arguments changed")
	}
}
