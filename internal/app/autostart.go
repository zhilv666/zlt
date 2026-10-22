package app

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
)

// autostartLog is the shared component-scoped logger for the platform-specific
// autostart implementations, so their entries are easy to filter in app.log.
func autostartLog() *slog.Logger {
	return slog.Default().With("component", "autostart")
}

type AutoStartStatus struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Status    string `json:"status"`
	UnitPath  string `json:"unit_path,omitempty"`
	Message   string `json:"message,omitempty"`
}

func autostartCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: autostart <enable|disable|status>")
	}

	switch args[0] {
	case "enable":
		if len(args) == 1 {
			return enableAutostart()
		}
		if runtime.GOOS != "linux" {
			return errors.New("custom autostart service options are supported only on Linux")
		}
		opts, err := parseServiceCommandArgs(args[1:])
		if err != nil {
			return err
		}
		return enableAutostartWithOptions(opts, true)
	case "disable":
		if len(args) != 1 {
			return errors.New("usage: autostart disable")
		}
		return disableAutostart()
	case "status":
		if len(args) != 1 {
			return errors.New("usage: autostart status")
		}
		return statusAutostart()
	default:
		return fmt.Errorf("unknown autostart action: %s", args[0])
	}
}

func GetAutoStartStatus() (AutoStartStatus, error) {
	return getAutoStartStatus()
}

func EnableAutoStart() error {
	return enableAutostart()
}

func DisableAutoStart() error {
	return disableAutostart()
}
