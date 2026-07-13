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
	// Detect if running under sudo - systemctl --user won't work
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		return fmt.Errorf("please run without sudo: systemctl --user requires your user session\nTry: ./zlt autostart enable --user")
	}

	exe, err := os.Executable()
	if err != nil {
		autostartLog().Error("enable: resolve executable failed", "err", err)
		return err
	}

	unitPath, err := systemdUnitPath()
	if err != nil {
		autostartLog().Error("enable: resolve unit path failed", "err", err)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		autostartLog().Error("enable: mkdir failed", "err", err)
		return err
	}

	workdir, err := os.Getwd()
	if err != nil {
		autostartLog().Error("enable: getwd failed", "err", err)
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
		autostartLog().Error("enable: write unit failed", "err", err)
		return err
	}
	autostartLog().Info("enabled", "unit", unitPath)

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
	// Detect if running under sudo - systemctl --user won't work
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		return fmt.Errorf("please run without sudo: systemctl --user requires your user session\nTry: ./zlt autostart disable --user")
	}

	unitPath, err := systemdUnitPath()
	if err != nil {
		autostartLog().Error("disable: resolve unit path failed", "err", err)
		return err
	}

	_ = runSystemctl("--user", "disable", "--now", systemdServiceName)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		autostartLog().Error("disable: remove unit failed", "err", err)
		return err
	}
	if err := runSystemctl("--user", "daemon-reload"); err != nil {
		return err
	}

	fmt.Println("autostart disabled")
	return nil
}

func statusAutostart() error {
	// Detect if running under sudo - systemctl --user won't work
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		return fmt.Errorf("please run without sudo: systemctl --user requires your user session\nTry: ./zlt autostart status --user")
	}

	status, err := getAutoStartStatus()
	if err != nil {
		autostartLog().Error("status query failed", "err", err)
		return err
	}
	autostartLog().Debug("status", "status", status.Status, "enabled", status.Enabled, "unit", status.UnitPath)
	fmt.Printf("autostart: %s\n", status.Status)
	return nil
}

func getAutoStartStatus() (AutoStartStatus, error) {
	unitPath, err := systemdUnitPath()
	if err != nil {
		return AutoStartStatus{}, err
	}

	if _, err := os.Stat(unitPath); err != nil {
		if os.IsNotExist(err) {
			return AutoStartStatus{
				Supported: true,
				Enabled:   false,
				Status:    "disabled",
				UnitPath:  unitPath,
			}, nil
		}
		return AutoStartStatus{}, err
	}

	cmd := exec.Command("systemctl", "--user", "is-enabled", systemdServiceName)
	output, err := cmd.CombinedOutput()
	status := strings.TrimSpace(string(output))
	autostartLog().Debug("is-enabled query", "command", strings.Join(cmd.Args, " "), "output", status, "err", err)
	if err != nil && status == "" {
		return AutoStartStatus{}, err
	}

	enabled := status == "enabled"
	return AutoStartStatus{
		Supported: true,
		Enabled:   enabled,
		Status:    status,
		UnitPath:  unitPath,
	}, nil
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
	autostartLog().Debug("systemctl", "command", strings.Join(cmd.Args, " "), "output", strings.TrimSpace(string(output)), "err", err)
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			return err
		}
		return fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}
