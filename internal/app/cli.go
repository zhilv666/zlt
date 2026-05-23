package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type RunOptions struct {
	Addr     string
	Headless bool
	PIDFile  string
}

func Execute(args []string) error {
	if len(args) == 0 {
		return Run()
	}

	switch args[0] {
	case "run":
		return runCommand(args[1:])
	case "start":
		return startCommand(args[1:])
	case "stop":
		return stopCommand(args[1:])
	case "restart":
		return restartCommand(args[1:])
	case "status":
		return statusCommand(args[1:])
	case "autostart":
		return autostartCommand(args[1:])
	default:
		if runtime.GOOS == "linux" {
			return runCommand(args)
		}
		return Run()
	}
}

func runCommand(args []string) error {
	opts := DefaultRunOptions()
	opts.Headless = true
	if len(args) > 0 && args[0] != "" {
		opts.Addr = args[0]
	}
	return RunWithOptions(opts)
}

func startCommand(args []string) error {
	pidFile := defaultPIDFile()
	if len(args) > 0 && args[0] != "" {
		pidFile = args[0]
	}

	if err := startDetached(pidFile); err != nil {
		return err
	}
	return nil
}

func stopCommand(args []string) error {
	pidFile := defaultPIDFile()
	if len(args) > 0 && args[0] != "" {
		pidFile = args[0]
	}
	return stopDetached(pidFile)
}

func restartCommand(args []string) error {
	pidFile := defaultPIDFile()
	if len(args) > 0 && args[0] != "" {
		pidFile = args[0]
	}
	if err := stopDetached(pidFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return startDetached(pidFile)
}

func statusCommand(args []string) error {
	pidFile := defaultPIDFile()
	if len(args) > 0 && args[0] != "" {
		pidFile = args[0]
	}

	lock, err := readPIDFile(pidFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("stopped")
			return nil
		}
		return err
	}
	defer lock.Release()

	fmt.Printf("running pid=%d addr=%s\n", lock.PID, defaultHTTPAddr)
	return nil
}

func defaultPIDFile() string {
	return filepath.Join("data", "tray.pid")
}
