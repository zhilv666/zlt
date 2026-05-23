package app

import (
	"errors"
	"fmt"
	"runtime"
)

type AutoStartStatus struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Status    string `json:"status"`
	UnitPath  string `json:"unit_path,omitempty"`
	Message   string `json:"message,omitempty"`
}

func autostartCommand(args []string) error {
	if runtime.GOOS != "linux" {
		return errors.New("autostart command is currently supported on linux only")
	}
	if len(args) == 0 {
		return errors.New("usage: autostart <enable|disable|status>")
	}

	switch args[0] {
	case "enable":
		return enableAutostart()
	case "disable":
		return disableAutostart()
	case "status":
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
