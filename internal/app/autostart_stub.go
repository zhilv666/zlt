//go:build !linux && !windows

package app

import "errors"

func enableAutostart() error {
	return errors.New("autostart is currently unsupported on this platform")
}

func disableAutostart() error {
	return errors.New("autostart is currently unsupported on this platform")
}

func statusAutostart() error {
	return errors.New("autostart is currently unsupported on this platform")
}

func getAutoStartStatus() (AutoStartStatus, error) {
	return AutoStartStatus{
		Supported: false,
		Enabled:   false,
		Status:    "unsupported",
		Message:   "autostart is currently unsupported on this platform",
	}, nil
}
