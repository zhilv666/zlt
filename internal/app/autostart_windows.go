//go:build windows

package app

import (
	"errors"
	"fmt"
	"log"
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
		log.Printf("autostart windows enable: resolve executable failed: %v", err)
		return err
	}

	workdir, err := os.Getwd()
	if err != nil {
		log.Printf("autostart windows enable: getwd failed: %v", err)
		return err
	}

	// Wrap paths in real double quotes WITHOUT Go-style escaping: the Windows
	// command-line parser does not unescape "\\", so %q's doubled backslashes
	// would corrupt the path and the entry would fail to launch at login.
	command := fmt.Sprintf(`"%s" --workdir "%s"`, exe, workdir)

	key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKeyPath, registry.SET_VALUE)
	if err != nil {
		log.Printf("autostart windows enable: open key failed: %v", err)
		return err
	}
	defer key.Close()

	if err := key.SetStringValue(windowsAutostartValue, command); err != nil {
		log.Printf("autostart windows enable: set value failed: %v", err)
		return err
	}
	log.Printf("autostart windows enable: set %s=%q", windowsAutostartValue, command)
	return nil
}

func disableAutostart() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKeyPath, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		log.Printf("autostart windows disable: open key failed: %v", err)
		return err
	}
	defer key.Close()

	if err := key.DeleteValue(windowsAutostartValue); err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		log.Printf("autostart windows disable: delete value failed: %v", err)
		return err
	}
	log.Printf("autostart windows disable: deleted %s", windowsAutostartValue)
	return nil
}

func statusAutostart() error {
	status, err := getAutoStartStatus()
	if err != nil {
		log.Printf("autostart windows status: err=%v", err)
		return err
	}
	log.Printf("autostart windows status: status=%s enabled=%v unit=%s", status.Status, status.Enabled, status.UnitPath)
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
		log.Printf("autostart windows status: open key failed: %v", err)
		return AutoStartStatus{}, err
	}
	defer key.Close()

	if _, _, err := key.GetStringValue(windowsAutostartValue); err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return AutoStartStatus{Supported: true, Enabled: false, Status: "disabled", UnitPath: unitPath}, nil
		}
		log.Printf("autostart windows status: get value failed: %v", err)
		return AutoStartStatus{}, err
	}

	return AutoStartStatus{Supported: true, Enabled: true, Status: "enabled", UnitPath: unitPath}, nil
}
