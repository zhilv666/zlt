package task

type Config struct {
	ID                          string   `json:"id"`
	Name                        string   `json:"name"`
	Program                     string   `json:"program"`
	Args                        []string `json:"args"`
	WorkDir                     string   `json:"workdir"`
	Env                         []string `json:"env"`
	AutoStart                   bool     `json:"autostart"`
	RestartOnCrash              bool     `json:"restart_on_crash"`
	StopTimeoutSec              int      `json:"stop_timeout_sec"`
	RestartDelaySec             int      `json:"restart_delay_sec"`
	MaxRestartCount             int      `json:"max_restart_count"`
	HealthCheckURL              string   `json:"health_check_url"`
	HealthCheckIntervalSec      int      `json:"health_check_interval_sec"`
	HealthCheckFailureThreshold int      `json:"health_check_failure_threshold"`
}

func DefaultOpenListTask() Config {
	return Config{
		ID:                          "openlist",
		Name:                        "OpenList",
		Program:                     "openlist.exe",
		Args:                        []string{"server"},
		WorkDir:                     ".",
		Env:                         []string{},
		AutoStart:                   false,
		RestartOnCrash:              false,
		StopTimeoutSec:              8,
		RestartDelaySec:             2,
		MaxRestartCount:             0,
		HealthCheckURL:              "",
		HealthCheckIntervalSec:      0,
		HealthCheckFailureThreshold: 0,
	}
}
