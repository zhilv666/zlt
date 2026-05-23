package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type pidLock struct {
	Path string
	PID  int
}

func acquirePIDFile(path string) (*pidLock, error) {
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if existing, err := readPIDFile(path); err == nil {
		if processExists(existing.PID) {
			return nil, fmt.Errorf("service already running with pid %d", existing.PID)
		}
		_ = os.Remove(path)
	}

	pid := os.Getpid()
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return nil, err
	}
	return &pidLock{Path: path, PID: pid}, nil
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
