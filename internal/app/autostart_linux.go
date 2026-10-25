//go:build linux

package app

import (
	"fmt"
	"os"
	"os/exec"
)

// initSystem identifies the service manager that owns autostart on this host.
// Linux is not limited to systemd: OpenWrt and its derivatives run BusyBox
// procd, which has its own /etc/init.d scripts and rc.d boot links.
type initSystem int

const (
	initUnknown initSystem = iota
	initSystemd
	initProcd
)

func (s initSystem) label() string {
	switch s {
	case initSystemd:
		return "systemd"
	case initProcd:
		return "procd"
	default:
		return "unknown"
	}
}

// autostartManager resolves the init system for the current process. Tests swap
// it out to exercise both backends without touching the host's real service
// configuration.
var autostartManager = func() initSystem {
	return detectInitSystem(os.Stat, exec.LookPath)
}

func detectInitSystem(stat func(string) (os.FileInfo, error), lookPath func(string) (string, error)) initSystem {
	// procd wins the check: OpenWrt images can carry stray systemd tools, but
	// /etc/rc.common + /sbin/procd is the authoritative combination there.
	if exists(stat, "/etc/rc.common") && exists(stat, "/sbin/procd") {
		return initProcd
	}
	if exists(stat, "/run/systemd/system") {
		return initSystemd
	}
	if _, err := lookPath("systemctl"); err == nil {
		return initSystemd
	}
	return initUnknown
}

func exists(stat func(string) (os.FileInfo, error), path string) bool {
	_, err := stat(path)
	return err == nil
}

func enableAutostart() error {
	return enableAutostartWithOptions(DefaultRunOptions(), false)
}

func enableAutostartWithOptions(opts RunOptions, update bool) error {
	switch autostartManager() {
	case initProcd:
		return enableProcdAutostart(opts, update)
	case initSystemd:
		return enableSystemdAutostart(opts, update)
	default:
		err := fmt.Errorf("no supported init system found (need systemd or OpenWrt procd)")
		autostartLog().Error("enable: detect init system failed", "err", err)
		return err
	}
}

func disableAutostart() error {
	switch autostartManager() {
	case initProcd:
		return disableProcdAutostart()
	case initSystemd:
		return disableSystemdAutostart()
	default:
		err := fmt.Errorf("no supported init system found (need systemd or OpenWrt procd)")
		autostartLog().Error("disable: detect init system failed", "err", err)
		return err
	}
}

func statusAutostart() error {
	status, err := getAutoStartStatus()
	if err != nil {
		autostartLog().Error("status query failed", "err", err)
		return err
	}
	autostartLog().Debug("status", "status", status.Status, "enabled", status.Enabled, "unit", status.UnitPath, "manager", status.Manager)
	fmt.Printf("autostart: %s (%s, %s)\n", status.Status, status.Manager, currentAutostartScope().label())
	return nil
}

func getAutoStartStatus() (AutoStartStatus, error) {
	manager := autostartManager()
	switch manager {
	case initProcd:
		status, err := procdAutostartStatus()
		status.Manager = manager.label()
		return status, err
	case initSystemd:
		status, err := systemdAutostartStatus()
		if err != nil {
			return status, err
		}
		status.Manager = manager.label()
		return status, nil
	default:
		return AutoStartStatus{
			Supported: false,
			Enabled:   false,
			Status:    "unsupported",
			Manager:   manager.label(),
			Message:   "no supported init system found (need systemd or OpenWrt procd)",
		}, nil
	}
}
