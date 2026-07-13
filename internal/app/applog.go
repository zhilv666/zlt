package app

import (
	"log/slog"
	"sync"

	"zhulingtai/internal/logging"
)

var (
	appLogOnce sync.Once
	// appLogHandle is retained so runtime settings changes (log level, app.log
	// rotation limits) can be applied to the live logger. It is nil until a
	// serving mode has initialized logging; all its methods are nil-safe.
	appLogHandle *logging.Handle
)

// initAppLogger installs the process-wide structured logger the first time a
// serving mode starts. It writes to data/app.log with size-based rotation, tees
// to the terminal when running in one, and honors ZLT_LOG_LEVEL (default info)
// as the bootstrap level until persisted settings are applied.
func initAppLogger() {
	appLogOnce.Do(func() {
		handle, err := logging.Setup(logging.Options{
			Level:   logging.LevelFromEnv(slog.LevelInfo),
			Console: true,
		})
		if err == nil {
			appLogHandle = handle
		}
	})
}
