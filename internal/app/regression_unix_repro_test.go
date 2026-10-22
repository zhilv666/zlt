//go:build unix

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRegressionDetachedStopStalePID(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() {
		_ = cmd.Process.Kill()
		select {
		case <-done:
		default:
		}
	}()
	path := filepath.Join(t.TempDir(), "zlt.pid")
	exe := "/tmp/zlt-regression-other-executable"
	if processMatches(cmd.Process.Pid, exe) {
		t.Fatal("fixture executable identity was not distinguished")
	}
	if _, err := writePIDFile(path, pidFilePayload{PID: cmd.Process.Pid, Exe: exe}); err != nil {
		t.Fatal(err)
	}
	if err := stopDetached(path); err != nil {
		t.Fatal(err)
	}
	select {
	case childErr := <-done:
		t.Fatalf("stopDetached signalled an unrelated executable despite the stored Exe mismatch: child=%v", childErr)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale payload not cleaned: %v", err)
	}
}

func TestRegressionDetachedStopLegacyPIDDoesNotKillOtherExecutable(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { _ = cmd.Process.Kill() }()
	path := filepath.Join(t.TempDir(), "zlt.pid")
	if _, err := writePIDFile(path, pidFilePayload{PID: cmd.Process.Pid}); err != nil {
		t.Fatal(err)
	}
	if err := stopDetached(path); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		t.Fatal("legacy payload caused an unrelated executable to be killed")
	case <-time.After(100 * time.Millisecond):
	}
}
