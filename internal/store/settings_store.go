package store

import (
	"database/sql"
	"errors"

	"zhulingtai/internal/task"
)

// Settings are stored as a single pinned row (id = 1). Defaults live in
// task.DefaultSettings so a fresh install (or a missing row) behaves exactly as
// the app did before settings were configurable.
func (s *TaskStore) initSettingsSchema() error {
	defaults := task.DefaultSettings()
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			log_level TEXT NOT NULL DEFAULT 'info',
			app_log_max_size_mb INTEGER NOT NULL DEFAULT 10,
			app_log_max_backups INTEGER NOT NULL DEFAULT 3,
			task_log_max_size_mb INTEGER NOT NULL DEFAULT 10,
			task_log_max_backups INTEGER NOT NULL DEFAULT 3
		)
	`); err != nil {
		return err
	}

	// Seed the single row on first run so LoadSettings always finds defaults.
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO settings (id, log_level, app_log_max_size_mb, app_log_max_backups, task_log_max_size_mb, task_log_max_backups)
		VALUES (1, ?, ?, ?, ?, ?)
	`, defaults.LogLevel, defaults.AppLogMaxSizeMB, defaults.AppLogMaxBackups, defaults.TaskLogMaxSizeMB, defaults.TaskLogMaxBackups)
	return err
}

func (s *TaskStore) LoadSettings() (task.Settings, error) {
	settings := task.DefaultSettings()
	err := s.db.QueryRow(`
		SELECT log_level, app_log_max_size_mb, app_log_max_backups, task_log_max_size_mb, task_log_max_backups
		FROM settings WHERE id = 1
	`).Scan(
		&settings.LogLevel,
		&settings.AppLogMaxSizeMB,
		&settings.AppLogMaxBackups,
		&settings.TaskLogMaxSizeMB,
		&settings.TaskLogMaxBackups,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return task.DefaultSettings(), nil
	}
	if err != nil {
		return task.Settings{}, err
	}
	return settings, nil
}

func (s *TaskStore) SaveSettings(settings task.Settings) error {
	_, err := s.db.Exec(`
		INSERT INTO settings (id, log_level, app_log_max_size_mb, app_log_max_backups, task_log_max_size_mb, task_log_max_backups)
		VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			log_level = excluded.log_level,
			app_log_max_size_mb = excluded.app_log_max_size_mb,
			app_log_max_backups = excluded.app_log_max_backups,
			task_log_max_size_mb = excluded.task_log_max_size_mb,
			task_log_max_backups = excluded.task_log_max_backups
	`,
		settings.LogLevel,
		settings.AppLogMaxSizeMB,
		settings.AppLogMaxBackups,
		settings.TaskLogMaxSizeMB,
		settings.TaskLogMaxBackups,
	)
	return err
}
