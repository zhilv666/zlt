package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// errAlreadyRunning is returned when the single-instance lock is already held by
// a live instance of this application. Callers use errors.Is to distinguish it
// from real I/O failures and exit gracefully instead of crashing.
var errAlreadyRunning = errors.New("another instance is already running")

type pidLock struct {
	Path string
	PID  int
	Addr string
	Exe  string
}

type pidFilePayload struct {
	PID  int    `json:"pid"`
	Addr string `json:"addr,omitempty"`
	Exe  string `json:"exe,omitempty"`
}

func acquirePIDFile(path string, addr string) (*pidLock, error) {
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if existing, err := readPIDFile(path); err == nil {
		if processMatches(existing.PID, existing.Exe) {
			return nil, fmt.Errorf("%w (pid %d)", errAlreadyRunning, existing.PID)
		}
		// Stale lock (process gone, or the pid was recycled by something else).
		_ = os.Remove(path)
	}

	exe, _ := os.Executable()
	payload := pidFilePayload{
		PID:  os.Getpid(),
		Addr: strings.TrimSpace(addr),
		Exe:  exe,
	}
	return writePIDFile(path, payload)
}

func writePIDFile(path string, payload pidFilePayload) (*pidLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	return &pidLock{Path: path, PID: payload.PID, Addr: payload.Addr, Exe: payload.Exe}, nil
}

func readPIDFile(path string) (*pidLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil, os.ErrNotExist
	}

	var payload pidFilePayload
	if strings.HasPrefix(text, "{") {
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, fmt.Errorf("invalid pid file: %s", path)
		}
		if payload.PID <= 0 {
			return nil, fmt.Errorf("invalid pid file: %s", path)
		}
		return &pidLock{Path: path, PID: payload.PID, Addr: strings.TrimSpace(payload.Addr), Exe: strings.TrimSpace(payload.Exe)}, nil
	}

	pid, err := strconv.Atoi(text)
	if err != nil || pid <= 0 {
		return nil, fmt.Errorf("invalid pid file: %s", path)
	}
	return &pidLock{Path: path, PID: pid}, nil
}

func (l *pidLock) Release() error {
	if l == nil || l.Path == "" {
		return nil
	}
	err := os.Remove(l.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func processExists(pid int) bool {
	return processExistsPlatform(pid)
}

// processMatches reports whether pid belongs to a live process that is still an
// instance of this application. The executable comparison defends against PID
// reuse: the recorded pid can be recycled by an unrelated process after an
// unclean exit or a reboot, and we must treat such a lock as stale.
func processMatches(pid int, exe string) bool {
	if !processExists(pid) {
		return false
	}
	if exe == "" {
		// Legacy pid file without an exe path: fall back to liveness only.
		return true
	}
	current, ok := processImagePath(pid)
	if !ok {
		// Image path unavailable on this platform: liveness only.
		return true
	}
	return sameExecutablePath(current, exe)
}

func sameExecutablePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
