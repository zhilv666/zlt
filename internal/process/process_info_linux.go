package process

import (
	"fmt"
	"os"
	"strings"
)

func readUnixProcessCommand(pid int) (string, []string, bool) {
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return "", nil, false
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(data) == 0 || data[len(data)-1] != 0 {
		return "", nil, false
	}
	argv := strings.Split(string(data[:len(data)-1]), "\x00")
	return exe, argv, true
}

func unixProcessMatchesWorkingDir(pid int, expected string) bool {
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	return err == nil && sameProcessFile(cwd, expected)
}
