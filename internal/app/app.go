package app

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"zhulingtai/internal/api"
	"zhulingtai/internal/buildinfo"
)

const defaultHTTPAddr = "127.0.0.1:3719"

func Run() error {
	return RunWithOptions(DefaultRunOptions())
}

func RunWithOptions(opts RunOptions) error {
	initAppLogger()

	// Single-instance guard: every serving mode (tray, foreground run, and the
	// detached daemon child) funnels through here, so acquiring the lock before
	// we touch the database or bind the HTTP port keeps a second launch from
	// clobbering the running instance. An empty PIDFile means "use the default",
	// so the plain `zlt` / double-click path is guarded too.
	lockPath := opts.PIDFile
	if lockPath == "" {
		lockPath = defaultPIDFile()
	}
	lock, err := acquirePIDFile(lockPath, opts.Addr)
	if err != nil {
		if errors.Is(err, errAlreadyRunning) {
			return handleAlreadyRunning(lockPath, opts)
		}
		return err
	}
	defer lock.Release()

	runtime, err := NewRuntime()
	if err != nil {
		return err
	}

	runtime.HTTP = newHTTPServer(runtime, opts.Addr)

	if err := runtime.StartHTTP(); err != nil {
		return err
	}

	if err := runtime.StartScheduler(); err != nil {
		slog.Error("scheduler start failed", "err", err)
	}

	slog.Info("zlt starting", "build", buildinfo.Summary())

	if opts.Headless {
		if err := runtime.StartAutoStartTasks(); err != nil {
			return err
		}
		return runHeadless(runtime)
	}
	return runTray(runtime)
}

// handleAlreadyRunning reacts to a launch that lost the single-instance race.
// A headless/daemon start returns a non-zero error so scripts notice; an
// interactive launch (tray / double-click) instead surfaces the instance that
// is already serving by opening its dashboard, then exits cleanly.
func handleAlreadyRunning(lockPath string, opts RunOptions) error {
	existing, err := readPIDFile(lockPath)
	if err != nil {
		return errAlreadyRunning
	}
	if opts.Headless {
		return fmt.Errorf("驻令台 已在运行 (pid %d)，请勿重复启动", existing.PID)
	}
	url := dashboardURL(existing.Addr)
	slog.Info("already running; opening existing dashboard", "pid", existing.PID, "url", url)
	openBrowser(url)
	return nil
}

func newHTTPServer(runtime *Runtime, addr string) *http.Server {
	apiServer := api.NewServer(runtime, runtime, autoStartAPIAdapter{}, runtime, runtime)
	return &http.Server{
		Addr:    addr,
		Handler: apiServer.Handler(),
	}
}

func DefaultRunOptions() RunOptions {
	return RunOptions{
		Addr:     defaultHTTPAddr,
		Headless: false,
		PIDFile:  "",
	}
}
