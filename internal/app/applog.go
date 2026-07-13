package app

import (
	"log/slog"
	"sync"

	"zhulingtai/internal/logging"
)

var appLogOnce sync.Once

// initAppLogger installs the process-wide structured logger the first time a
// serving mode starts. It writes to data/app.log with size-based rotation, tees
// to the terminal when running in one, and honors ZLT_LOG_LEVEL (default info).
func initAppLogger() {
	appLogOnce.Do(func() {
		_, _ = logging.Setup(logging.Options{
			Level:   logging.LevelFromEnv(slog.LevelInfo),
			Console: true,
		})
	})
}
