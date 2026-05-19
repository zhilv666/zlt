package task

type Config struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Program        string   `json:"program"`
	Args           []string `json:"args"`
	WorkDir        string   `json:"workdir"`
	Env            []string `json:"env"`
	AutoStart      bool     `json:"autostart"`
	RestartOnCrash bool     `json:"restart_on_crash"`
	StopTimeoutSec int      `json:"stop_timeout_sec"`
}

func DefaultOpenListTask() Config {
	return Config{
		ID:             "openlist",
		Name:           "OpenList",
		Program:        "openlist.exe",
		Args:           []string{"server"},
		WorkDir:        ".",
		Env:            []string{},
		AutoStart:      false,
		RestartOnCrash: false,
		StopTimeoutSec: 8,
	}
}
