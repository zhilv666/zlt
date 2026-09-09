package app

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zhulingtai/internal/api"
	"zhulingtai/internal/auth"
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

	authSvc, keyPath, sessions, err := initAuth()
	if err != nil {
		return err
	}
	runtime.Auth = authSvc
	runtime.AuthKeyPath = keyPath
	runtime.authSessions = sessions

	runtime.HTTP = newHTTPServer(runtime, opts.Addr, authSvc)

	if err := runtime.StartHTTP(); err != nil {
		return err
	}

	// Purge expired sessions now (the server may have been down long enough for
	// sessions to lapse), then sweep hourly to keep the table bounded. The
	// hourly sweep also lets a rotated key clean up stale rows lazily.
	if n, err := authSvc.PurgeExpired(); err != nil {
		slog.Warn("session purge failed", "err", err)
	} else if n > 0 {
		slog.Info("purged expired sessions", "count", n)
	}
	go runtime.purgeSessionsLoop()

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
	// Prefer the configured public URL (ZLT_PUBLIC_URL) so a behind-proxy
	// deployment opens the real address; fall back to the listener addr.
	if pub := strings.TrimSpace(os.Getenv("ZLT_PUBLIC_URL")); pub != "" {
		slog.Info("already running; opening existing dashboard", "pid", existing.PID, "url", pub)
		openBrowser(pub)
		return nil
	}
	url := dashboardURL(existing.Addr)
	slog.Info("already running; opening existing dashboard", "pid", existing.PID, "url", url)
	openBrowser(url)
	return nil
}

// initAuth loads (or generates) the access key, opens the session database and
// constructs the auth Service. A failure here is startup-fatal: a missing or
// corrupt key is never silently replaced, and a bad public URL is rejected
// rather than silently downgrading to insecure cookies.
func initAuth() (svc *auth.Service, keyPath string, sessions *auth.SessionStore, err error) {
	keyPath = filepath.Join("data", "auth.key")
	key, err := auth.LoadOrCreateKey(keyPath)
	if err != nil {
		return nil, "", nil, err
	}
	sessions, err = auth.NewSessionStore(filepath.Join("data", "auth.db"))
	if err != nil {
		return nil, "", nil, err
	}
	svc, err = auth.NewService(auth.Config{
		Key:            key,
		Sessions:       sessions,
		PublicURL:      strings.TrimSpace(os.Getenv("ZLT_PUBLIC_URL")),
		TrustedProxies: splitCSV(os.Getenv("ZLT_TRUSTED_PROXIES")),
	})
	if err != nil {
		_ = sessions.Close()
		return nil, "", nil, err
	}
	return svc, keyPath, sessions, nil
}

// splitCSV trims and drops empty entries from a comma-separated env value.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// purgeSessionsLoop periodically drops sessions whose idle lifetime has passed.
// It runs in the background for the lifetime of the process; Shutdown does not
// need to stop it explicitly — process exit cancels it.
func (r *Runtime) purgeSessionsLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if r.Auth == nil {
			return
		}
		if n, err := r.Auth.PurgeExpired(); err != nil {
			slog.Warn("session purge failed", "err", err)
		} else if n > 0 {
			slog.Info("purged expired sessions", "count", n)
		}
	}
}

// DashboardURL returns the URL the tray and "already running" path should open:
// the configured public https URL when set, otherwise the local http listener.
func (r *Runtime) DashboardURL() string {
	if r.Auth != nil {
		if pub := r.Auth.PublicURL(); pub != "" {
			return pub
		}
	}
	return r.Address()
}

func newHTTPServer(runtime *Runtime, addr string, authSvc *auth.Service) *http.Server {
	apiServer := api.NewServer(runtime, runtime, autoStartAPIAdapter{}, runtime, runtime)
	if authSvc != nil {
		apiServer.WithAuth(authSvc)
	}
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
