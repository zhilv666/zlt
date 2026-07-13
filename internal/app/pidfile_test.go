package app

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func mustExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve executable: %v", err)
	}
	return exe
}

// TestProcessExistsSelf is a regression guard: on Windows the previous
// Signal(0) implementation reported the current (obviously live) process as
// dead, which silently disabled the single-instance guard.
func TestProcessExistsSelf(t *testing.T) {
	if !processExists(os.Getpid()) {
		t.Fatal("processExists must report the current process as alive")
	}
	if processExists(-1) || processExists(0) {
		t.Fatal("processExists must reject non-positive pids")
	}
}

func TestProcessImagePathMatchesSelf(t *testing.T) {
	got, ok := processImagePath(os.Getpid())
	if !ok {
		t.Skip("process image path not available on this platform")
	}
	if !sameExecutablePath(got, mustExecutable(t)) {
		t.Fatalf("image path %q does not match executable %q", got, mustExecutable(t))
	}
}

func TestAcquirePIDFileWritesPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zlt.pid")

	lock, err := acquirePIDFile(path, "127.0.0.1:9999")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer lock.Release()

	if lock.PID != os.Getpid() {
		t.Fatalf("lock pid = %d, want %d", lock.PID, os.Getpid())
	}

	saved, err := readPIDFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if saved.PID != os.Getpid() {
		t.Fatalf("saved pid = %d, want %d", saved.PID, os.Getpid())
	}
	if saved.Addr != "127.0.0.1:9999" {
		t.Fatalf("saved addr = %q, want %q", saved.Addr, "127.0.0.1:9999")
	}
	if !sameExecutablePath(saved.Exe, mustExecutable(t)) {
		t.Fatalf("saved exe = %q, want %q", saved.Exe, mustExecutable(t))
	}
}

func TestAcquirePIDFileRejectsRunningInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zlt.pid")

	first, err := acquirePIDFile(path, "")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.Release()

	_, err = acquirePIDFile(path, "")
	if !errors.Is(err, errAlreadyRunning) {
		t.Fatalf("second acquire error = %v, want errAlreadyRunning", err)
	}
}

// TestAcquirePIDFileReplacesReusedPID covers the PID-reuse defense: a lock that
// points at a live pid backed by a *different* executable must be treated as
// stale rather than blocking startup forever.
func TestAcquirePIDFileReplacesReusedPID(t *testing.T) {
	if _, ok := processImagePath(os.Getpid()); !ok {
		t.Skip("process image path not available; cannot exercise PID-reuse defense")
	}
	path := filepath.Join(t.TempDir(), "zlt.pid")

	// Live pid, but recorded against an executable that is definitely not us.
	if _, err := writePIDFile(path, pidFilePayload{
		PID: os.Getpid(),
		Exe: filepath.Join(t.TempDir(), "not-zlt-binary"),
	}); err != nil {
		t.Fatalf("seed stale pid file: %v", err)
	}

	lock, err := acquirePIDFile(path, "")
	if err != nil {
		t.Fatalf("acquire over reused pid should succeed, got %v", err)
	}
	defer lock.Release()

	saved, err := readPIDFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !sameExecutablePath(saved.Exe, mustExecutable(t)) {
		t.Fatalf("lock was not reclaimed: exe = %q", saved.Exe)
	}
}

// TestAcquirePIDFileLegacyPlainInteger ensures pid files written by older
// versions (a bare integer, no exe) still guard via a liveness-only check.
func TestAcquirePIDFileLegacyPlainInteger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zlt.pid")
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatalf("write legacy pid file: %v", err)
	}

	saved, err := readPIDFile(path)
	if err != nil {
		t.Fatalf("read legacy: %v", err)
	}
	if saved.PID != os.Getpid() || saved.Exe != "" {
		t.Fatalf("legacy parse: pid=%d exe=%q", saved.PID, saved.Exe)
	}

	_, err = acquirePIDFile(path, "")
	if !errors.Is(err, errAlreadyRunning) {
		t.Fatalf("legacy live pid should block startup, got %v", err)
	}
}

func TestDashboardURL(t *testing.T) {
	if got := dashboardURL(""); got != "http://"+defaultHTTPAddr {
		t.Fatalf("empty addr: got %q", got)
	}
	if got := dashboardURL("0.0.0.0:8080"); got != "http://0.0.0.0:8080" {
		t.Fatalf("explicit addr: got %q", got)
	}
}
