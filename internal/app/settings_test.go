package app

import (
	"testing"

	"zhulingtai/internal/task"
)

func TestNormalizeSettingsClampsAndValidates(t *testing.T) {
	// Valid level (trimmed/lowercased) with out-of-range numbers that must clamp.
	got, err := normalizeSettings(task.Settings{
		LogLevel:          "  INFO ",
		AppLogMaxSizeMB:   0,     // -> 1 (min)
		AppLogMaxBackups:  -5,    // -> 0 (min)
		TaskLogMaxSizeMB:  99999, // -> 1024 (max)
		TaskLogMaxBackups: 3,     // unchanged
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := task.Settings{
		LogLevel:          "info",
		AppLogMaxSizeMB:   1,
		AppLogMaxBackups:  0,
		TaskLogMaxSizeMB:  1024,
		TaskLogMaxBackups: 3,
	}
	if got != want {
		t.Fatalf("normalized = %+v, want %+v", got, want)
	}
}

func TestNormalizeSettingsRejectsBadLevel(t *testing.T) {
	_, err := normalizeSettings(task.Settings{
		LogLevel:          "trace",
		AppLogMaxSizeMB:   10,
		AppLogMaxBackups:  3,
		TaskLogMaxSizeMB:  10,
		TaskLogMaxBackups: 3,
	})
	if err == nil {
		t.Fatal("expected an error for an invalid log level")
	}
}
