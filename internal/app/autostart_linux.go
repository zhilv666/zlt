//go:build linux

package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const systemdServiceName = "zhulingtai.service"

func enableAutostart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	unitPath, err := systemdUnitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return err
	}

	workdir, err := os.Getwd()
	if err != nil {
		return err
	}

	content := strings.TrimSpace(fmt.Sprintf(`
[Unit]
Description=驻令台
After=network.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s run
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
`, workdir, exe)) + "\n"

	if err := os.WriteFile(unitPath, []byte(content), 0o644); err != nil {
		return err
	}

	if err := runSystemctl("--user", "daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctl("--user", "enable", "--now", systemdServiceName); err != nil {
		return err
	}

	fmt.Printf("autostart enabled: %s\n", unitPath)
	return nil
}

func disableAutostart() error {
	unitPath, err := systemdUnitPath()
	if err != nil {
		return err
	}

	_ = runSystemctl("--user", "disable", "--now", systemdServiceName)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := runSystemctl("--user", "daemon-reload"); err != nil {
		return err
	}

	fmt.Println("autostart disabled")
	return nil
}

func statusAutostart() error {
	unitPath, err := systemdUnitPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(unitPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("autostart: disabled")
			return nil
		}
		return err
	}

	cmd := exec.Command("systemctl", "--user", "is-enabled", systemdServiceName)
	output, err := cmd.CombinedOutput()
	status := strings.TrimSpace(string(output))
	if err != nil && status == "" {
		return err
	}

	fmt.Printf("autostart: %s\n", status)
	return nil
}

func systemdUnitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", systemdServiceName), nil
}

func runSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			return err
		}
		return fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}
