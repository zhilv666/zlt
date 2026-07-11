package app

import (
	"log"
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

	runtime, err := NewRuntime()
	if err != nil {
		return err
	}

	var lock *pidLock
	if opts.PIDFile != "" {
		lock, err = acquirePIDFile(opts.PIDFile, opts.Addr)
		if err != nil {
			return err
		}
		defer lock.Release()
	}

	runtime.HTTP = newHTTPServer(runtime, opts.Addr)

	if err := runtime.StartHTTP(); err != nil {
		return err
	}

	if err := runtime.StartScheduler(); err != nil {
		log.Printf("scheduler start: %v", err)
	}

	log.Printf("zlt starting: %s", buildinfo.Summary())

	if opts.Headless {
		if err := runtime.StartAutoStartTasks(); err != nil {
			return err
		}
		return runHeadless(runtime)
	}
	return runTray(runtime)
}

func newHTTPServer(runtime *Runtime, addr string) *http.Server {
	apiServer := api.NewServer(runtime, runtime, autoStartAPIAdapter{}, runtime)
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
