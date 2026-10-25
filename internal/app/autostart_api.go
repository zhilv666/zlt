package app

import "zhulingtai/internal/api"

type autoStartAPIAdapter struct{ addr string }

func (autoStartAPIAdapter) Status() (api.AutoStartStatus, error) {
	status, err := GetAutoStartStatus()
	if err != nil {
		return api.AutoStartStatus{}, err
	}
	return api.AutoStartStatus{
		Supported: status.Supported,
		Enabled:   status.Enabled,
		Status:    status.Status,
		Manager:   status.Manager,
		UnitPath:  status.UnitPath,
		Message:   status.Message,
	}, nil
}

func (a autoStartAPIAdapter) Enable() error {
	opts := DefaultRunOptions()
	if a.addr != "" {
		opts.Addr = a.addr
	}
	return enableAutostartWithOptions(opts, false)
}

func (autoStartAPIAdapter) Disable() error {
	return DisableAutoStart()
}
