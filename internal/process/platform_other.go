//go:build !windows

package process

import (
	"fmt"
	"os"
	"os/exec"

	"tray/internal/task"
)

func prepareCommand(cmd *exec.Cmd) {}

func killProcessTree(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func findExistingProcess(cfg task.Config) (int, bool) {
	return 0, false
}
