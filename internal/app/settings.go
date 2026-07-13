package app

import (
	"fmt"
	"strings"

	"zhulingtai/internal/logging"
	"zhulingtai/internal/task"
)

const megabyte = 1024 * 1024

// Settings returns the persisted application settings (currently logging).
func (r *Runtime) Settings() (task.Settings, error) {
	return r.TaskStore.LoadSettings()
}

// UpdateSettings validates and persists new settings, then applies them live.
// The normalized settings that were actually stored are returned so the caller
// (and the UI) reflects any clamping.
func (r *Runtime) UpdateSettings(in task.Settings) (task.Settings, error) {
	normalized, err := normalizeSettings(in)
	if err != nil {
		return task.Settings{}, err
	}
	if err := r.TaskStore.SaveSettings(normalized); err != nil {
		return task.Settings{}, err
	}
	r.applySettings(normalized)
	return normalized, nil
}

// applySettings pushes settings into the live logger and process manager. It is
// safe before logging is initialized (appLogHandle methods are nil-safe) and
// before the manager exists.
func (r *Runtime) applySettings(s task.Settings) {
	if level, ok := logging.ParseLevel(s.LogLevel); ok {
		appLogHandle.SetLevel(level)
	}
	appLogHandle.SetAppLogLimits(int64(s.AppLogMaxSizeMB)*megabyte, s.AppLogMaxBackups)
	if r.Manager != nil {
		r.Manager.SetLogLimits(int64(s.TaskLogMaxSizeMB)*megabyte, s.TaskLogMaxBackups)
	}
}

// normalizeSettings trims/validates the log level and clamps the numeric limits
// to sane bounds. An unrecognized level is a hard error; out-of-range numbers
// are clamped so a typo can never wedge logging.
func normalizeSettings(s task.Settings) (task.Settings, error) {
	level := strings.ToLower(strings.TrimSpace(s.LogLevel))
	if _, ok := logging.ParseLevel(level); !ok {
		return task.Settings{}, fmt.Errorf("invalid log level %q (want debug|info|warn|error)", s.LogLevel)
	}
	s.LogLevel = level
	s.AppLogMaxSizeMB = clampInt(s.AppLogMaxSizeMB, 1, 1024)
	s.AppLogMaxBackups = clampInt(s.AppLogMaxBackups, 0, 100)
	s.TaskLogMaxSizeMB = clampInt(s.TaskLogMaxSizeMB, 1, 1024)
	s.TaskLogMaxBackups = clampInt(s.TaskLogMaxBackups, 0, 100)
	return s, nil
}

func clampInt(value, lo, hi int) int {
	if value < lo {
		return lo
	}
	if value > hi {
		return hi
	}
	return value
}
