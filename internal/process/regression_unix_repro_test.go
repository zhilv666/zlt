//go:build unix

package process

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"zhulingtai/internal/task"
)

func TestRegressionAdoptedProcessStop(t *testing.T) {
	cfg := regressionTask(t, "hold")
	cmd := exec.Command(cfg.Program, cfg.Args...)
	cmd.Dir = cfg.WorkDir
	cmd.Env = append(os.Environ(), cfg.Env...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { _ = cmd.Process.Kill() }()
	if err := syscall.Kill(-cmd.Process.Pid, 0); err != syscall.ESRCH {
		t.Skip("fixture unexpectedly owns a process group")
	}
	m := NewManager([]task.Config{cfg})
	state, _ := m.State(cfg.ID)
	if state.Status != StatusRunning || state.PID != cmd.Process.Pid {
		t.Fatalf("fixture was not adopted: child=%d state=%+v", cmd.Process.Pid, state)
	}
	if err := m.Stop(cfg.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		state, _ = m.State(cfg.ID)
		t.Fatalf("Stop reported success and state=%s, but adopted child PID=%d is still alive", state.Status, cmd.Process.Pid)
	}
}
