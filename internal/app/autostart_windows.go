//go:build windows

package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const windowsRunKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const windowsAutostartValue = "驻令台"

func windowsRunKeyDisplay() string {
	return filepath.Join("HKCU", "Software", "Microsoft", "Windows", "CurrentVersion", "Run")
}

func enableAutostart() error {
	exe, err := os.Executable()
	if err != nil {
		autostartLog().Error("enable: resolve executable failed", "err", err)
		return err
	}

	workdir, err := os.Getwd()
	if err != nil {
		autostartLog().Error("enable: getwd failed", "err", err)
		return err
	}

	// Wrap paths in real double quotes WITHOUT Go-style escaping: the Windows
	// command-line parser does not unescape "\\", so %q's doubled backslashes
	// would corrupt the path and the entry would fail to launch at login.
	command := fmt.Sprintf(`"%s" --workdir "%s"`, exe, workdir)

	key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKeyPath, registry.SET_VALUE)
	if err != nil {
		autostartLog().Error("enable: open registry key failed", "err", err)
		return err
	}
	defer key.Close()

	if err := key.SetStringValue(windowsAutostartValue, command); err != nil {
		autostartLog().Error("enable: set registry value failed", "err", err)
		return err
	}
	autostartLog().Info("enabled", "value", windowsAutostartValue, "command", command)
	return nil
}

func disableAutostart() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKeyPath, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		autostartLog().Error("disable: open registry key failed", "err", err)
		return err
	}
	defer key.Close()

	if err := key.DeleteValue(windowsAutostartValue); err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		autostartLog().Error("disable: delete registry value failed", "err", err)
		return err
	}
	autostartLog().Info("disabled", "value", windowsAutostartValue)
	return nil
}

func statusAutostart() error {
	status, err := getAutoStartStatus()
	if err != nil {
		autostartLog().Error("status query failed", "err", err)
		return err
	}
	autostartLog().Debug("status", "status", status.Status, "enabled", status.Enabled, "unit", status.UnitPath)
	fmt.Printf("autostart: %s\n", status.Status)
	return nil
}

func getAutoStartStatus() (AutoStartStatus, error) {
	unitPath := windowsRunKeyDisplay()

	key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKeyPath, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return AutoStartStatus{Supported: true, Enabled: false, Status: "disabled", UnitPath: unitPath}, nil
		}
		autostartLog().Error("status: open registry key failed", "err", err)
		return AutoStartStatus{}, err
	}
	defer key.Close()

	if _, _, err := key.GetStringValue(windowsAutostartValue); err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return AutoStartStatus{Supported: true, Enabled: false, Status: "disabled", UnitPath: unitPath}, nil
		}
		autostartLog().Error("status: read registry value failed", "err", err)
		return AutoStartStatus{}, err
	}

	return AutoStartStatus{Supported: true, Enabled: true, Status: "enabled", UnitPath: unitPath}, nil
}
