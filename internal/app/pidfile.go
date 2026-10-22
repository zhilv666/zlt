package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// errAlreadyRunning is returned when the single-instance lock is already held by
// a live instance of this application. Callers use errors.Is to distinguish it
// from real I/O failures and exit gracefully instead of crashing.
var errAlreadyRunning = errors.New("another instance is already running")

type pidLock struct {
	Path  string
	PID   int
	Addr  string
	Exe   string
	Token string
	file  *os.File
	mu    sync.Mutex
}

type pidFilePayload struct {
	PID   int    `json:"pid"`
	Addr  string `json:"addr,omitempty"`
	Exe   string `json:"exe,omitempty"`
	Token string `json:"token,omitempty"`
}

func acquirePIDFile(path string, addr string) (*pidLock, error) {
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// Keep the sidecar inode in place: deleting a locked file would let another
	// process lock a new inode at the same path while this instance still owns it.
	file, err := lockPIDFile(path + ".lock")
	if err != nil {
		return nil, err
	}
	owned := false
	defer func() {
		if !owned {
			_ = file.Close()
		}
	}()
	if existing, err := readPIDFile(path); err == nil {
		if processMatches(existing.PID, existing.Exe) {
			return nil, fmt.Errorf("%w (pid %d)", errAlreadyRunning, existing.PID)
		}
	}

	exe, _ := os.Executable()
	payload := pidFilePayload{
		PID:  os.Getpid(),
		Addr: strings.TrimSpace(addr),
		Exe:  exe,
	}
	lock, err := writePIDFile(path, payload)
	if err != nil {
		return nil, err
	}
	lock.file = file
	owned = true
	return lock, nil
}

func writePIDFile(path string, payload pidFilePayload) (*pidLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if payload.Token == "" {
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			return nil, err
		}
		payload.Token = hex.EncodeToString(token[:])
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".zlt-pid-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return nil, err
	}
	return &pidLock{Path: path, PID: payload.PID, Addr: payload.Addr, Exe: payload.Exe, Token: payload.Token}, nil
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
		return &pidLock{Path: path, PID: payload.PID, Addr: strings.TrimSpace(payload.Addr), Exe: strings.TrimSpace(payload.Exe), Token: payload.Token}, nil
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
	l.mu.Lock()
	defer l.mu.Unlock()
	file := l.file
	if file == nil {
		var err error
		file, err = lockPIDFile(l.Path + ".lock")
		if err != nil {
			return err
		}
	}
	defer func() { _ = file.Close(); l.file = nil }()
	current, err := readPIDFile(l.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.PID != l.PID || current.Exe != l.Exe || current.Token != l.Token {
		return nil
	}
	err = os.Remove(l.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func processExists(pid int) bool {
	return processExistsPlatform(pid)
}

// Stopping requires a verified identity. For legacy payloads, compare against
// the current application's executable; an unreadable image is an error.
func detachedProcessMatches(lock *pidLock) (bool, error) {
	if !processExists(lock.PID) {
		return false, nil
	}
	expected := lock.Exe
	if expected == "" {
		var err error
		expected, err = os.Executable()
		if err != nil {
			return false, err
		}
	}
	current, ok := processImagePath(lock.PID)
	if !ok {
		if !processExists(lock.PID) {
			return false, nil
		}
		return false, fmt.Errorf("cannot verify executable of service pid %d", lock.PID)
	}
	return sameExecutablePath(current, expected), nil
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
