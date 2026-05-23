package app

import (
	"errors"
	"fmt"
	"runtime"
)

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
