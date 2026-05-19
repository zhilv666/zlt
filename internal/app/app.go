package app

import (
	"net/http"

	"tray/internal/api"
)

func Run() error {
	runtime, err := NewRuntime()
	if err != nil {
		return err
	}

	runtime.HTTP = newHTTPServer(runtime)

	if err := runtime.StartHTTP(); err != nil {
		return err
	}

	return runTray(runtime)
}

func newHTTPServer(runtime *Runtime) *http.Server {
	apiServer := api.NewServer(runtime, runtime.Manager)
	return &http.Server{
		Addr:    "127.0.0.1:3719",
		Handler: apiServer.Handler(),
	}
}
