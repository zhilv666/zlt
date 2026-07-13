package logging

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Options configures Setup. Zero values fall back to sensible defaults.
type Options struct {
	Dir        string     // directory for the log file (default "data")
	FileName   string     // log file name (default "app.log")
	MaxSizeMB  int        // rotate threshold in MiB (default 10)
	MaxBackups int        // rotated files to keep (default 3)
	Level      slog.Level // minimum level (default from LevelFromEnv)
	Console    bool       // also write to stderr when it is a terminal
}

// Handle owns the process-wide log sink and lets callers adjust the level and
// rotation limits at runtime (e.g. from the settings API). Close is optional
// (the OS reclaims the file on exit); it exists mainly for tests.
type Handle struct {
	writer *RotatingWriter
	level  *slog.LevelVar
}

func (h *Handle) Close() error {
	if h == nil || h.writer == nil {
		return nil
	}
	return h.writer.Close()
}

// SetLevel changes the minimum log level live; all subsequent records honor it.
func (h *Handle) SetLevel(level slog.Level) {
	if h == nil || h.level == nil {
		return
	}
	h.level.Set(level)
}

// SetAppLogLimits changes app.log rotation limits live. A size of 0 disables
// rotation; the change takes effect on the next write.
func (h *Handle) SetAppLogLimits(maxSize int64, maxBackups int) {
	if h == nil || h.writer == nil {
		return
	}
	h.writer.SetLimits(maxSize, maxBackups)
}

// Setup builds a leveled, structured slog logger that writes to a rotating file
// (optionally teed to the terminal) and installs it as the default logger. It
// also redirects the standard library logger to the same sink so stray log.*
// output (e.g. from the cron engine) still lands in the app log.
func Setup(opts Options) (*Handle, error) {
	dir := opts.Dir
	if dir == "" {
		dir = "data"
	}
	name := opts.FileName
	if name == "" {
		name = "app.log"
	}
	maxMB := opts.MaxSizeMB
	if maxMB <= 0 {
		maxMB = 10
	}
	backups := opts.MaxBackups
	if backups <= 0 {
		backups = 3
	}

	writer, err := NewRotatingWriter(filepath.Join(dir, name), int64(maxMB)*1024*1024, backups)
	if err != nil {
		return nil, err
	}

	var out io.Writer = writer
	if opts.Console && isTerminal(os.Stderr) {
		out = io.MultiWriter(writer, os.Stderr)
	}

	// A LevelVar lets the settings API raise or lower verbosity without rebuilding
	// the handler.
	level := new(slog.LevelVar)
	level.Set(opts.Level)

	handler := slog.NewTextHandler(out, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: compactTime,
	})
	slog.SetDefault(slog.New(handler))

	// Capture anything still using the standard logger into the same file.
	log.SetOutput(out)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	return &Handle{writer: writer, level: level}, nil
}

// LevelFromEnv resolves ZLT_LOG_LEVEL (debug/info/warn/error), falling back to
// the given default when unset or unrecognized.
func LevelFromEnv(fallback slog.Level) slog.Level {
	if level, ok := ParseLevel(os.Getenv("ZLT_LOG_LEVEL")); ok {
		return level
	}
	return fallback
}

// ParseLevel maps a level name (debug/info/warn/error, case-insensitive) to a
// slog.Level. ok is false when s is empty or unrecognized.
func ParseLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

// LevelString renders a slog.Level as its lowercase name.
func LevelString(level slog.Level) string {
	switch {
	case level <= slog.LevelDebug:
		return "debug"
	case level < slog.LevelWarn:
		return "info"
	case level < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}

// compactTime renders the top-level time attribute as a short local timestamp
// instead of slog's default RFC3339, keeping app.log easy to skim.
func compactTime(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		if t, ok := a.Value.Any().(time.Time); ok {
			a.Value = slog.StringValue(t.Format("2006/01/02 15:04:05.000"))
		}
	}
	return a
}

// isTerminal reports whether f is a character device (a console) rather than a
// file or pipe. Used to decide whether teeing logs to stderr is useful — it is
// for `zlt run` in a terminal, but not for the tray or the detached daemon
// (whose stderr is redirected to a file we must not grow unbounded).
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
