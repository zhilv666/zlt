package app

import (
	"log"
	"net/http"

	"tray/internal/api"
	"tray/internal/buildinfo"
)

const defaultHTTPAddr = "127.0.0.1:3719"

func Run() error {
	return RunWithOptions(DefaultRunOptions())
}

func RunWithOptions(opts RunOptions) error {
	runtime, err := NewRuntime()
	if err != nil {
		return err
	}

	var lock *pidLock
	if opts.PIDFile != "" {
		lock, err = acquirePIDFile(opts.PIDFile)
		if err != nil {
			return err
		}
		defer lock.Release()
	}

	runtime.HTTP = newHTTPServer(runtime, opts.Addr)

	if err := runtime.StartHTTP(); err != nil {
		return err
	}

	log.Printf("tray starting: %s", buildinfo.Summary())

	if opts.Headless {
		return runHeadless(runtime)
	}
	return runTray(runtime)
}

func newHTTPServer(runtime *Runtime, addr string) *http.Server {
	apiServer := api.NewServer(runtime, runtime)
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
