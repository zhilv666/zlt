package app

import "tray/internal/api"

type autoStartAPIAdapter struct{}

func (autoStartAPIAdapter) Status() (api.AutoStartStatus, error) {
	status, err := GetAutoStartStatus()
	if err != nil {
		return api.AutoStartStatus{}, err
	}
	return api.AutoStartStatus{
		Supported: status.Supported,
		Enabled:   status.Enabled,
		Status:    status.Status,
		UnitPath:  status.UnitPath,
		Message:   status.Message,
	}, nil
}

func (autoStartAPIAdapter) Enable() error {
	return EnableAutoStart()
}

func (autoStartAPIAdapter) Disable() error {
	return DisableAutoStart()
}
