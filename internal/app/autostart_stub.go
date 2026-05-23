//go:build !linux

package app

import "errors"

func enableAutostart() error {
	return errors.New("autostart is currently supported on linux only")
}

func disableAutostart() error {
	return errors.New("autostart is currently supported on linux only")
}

func statusAutostart() error {
	return errors.New("autostart is currently supported on linux only")
}
