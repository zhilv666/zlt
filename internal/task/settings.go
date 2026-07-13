package task

// Settings holds user-adjustable application configuration exposed in the web UI
// (currently the logging options). It is persisted as a single row by the store.
type Settings struct {
	// LogLevel is the minimum slog level for app.log: debug | info | warn | error.
	LogLevel string `json:"log_level"`
	// AppLogMaxSizeMB / AppLogMaxBackups control rotation of the application log
	// (data/app.log).
	AppLogMaxSizeMB  int `json:"app_log_max_size_mb"`
	AppLogMaxBackups int `json:"app_log_max_backups"`
	// TaskLogMaxSizeMB / TaskLogMaxBackups control rotation of each managed task's
	// captured output (data/logs/<id>/app.log).
	TaskLogMaxSizeMB  int `json:"task_log_max_size_mb"`
	TaskLogMaxBackups int `json:"task_log_max_backups"`
}

// DefaultSettings returns the built-in defaults, matching the values the app
// used before settings were configurable.
func DefaultSettings() Settings {
	return Settings{
		LogLevel:          "info",
		AppLogMaxSizeMB:   10,
		AppLogMaxBackups:  3,
		TaskLogMaxSizeMB:  10,
		TaskLogMaxBackups: 3,
	}
}
