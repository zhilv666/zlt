//go:build linux

package app

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const systemdServiceName = "zhulingtai.service"

func enableAutostart() error {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("autostart linux enable: resolve executable failed: %v", err)
		return err
	}

	unitPath, err := systemdUnitPath()
	if err != nil {
		log.Printf("autostart linux enable: unit path failed: %v", err)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		log.Printf("autostart linux enable: mkdir failed: %v", err)
		return err
	}

	workdir, err := os.Getwd()
	if err != nil {
		log.Printf("autostart linux enable: getwd failed: %v", err)
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
		log.Printf("autostart linux enable: write unit failed: %v", err)
		return err
	}
	log.Printf("autostart linux enable: wrote unit=%s", unitPath)

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
		log.Printf("autostart linux disable: unit path failed: %v", err)
		return err
	}

	_ = runSystemctl("--user", "disable", "--now", systemdServiceName)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		log.Printf("autostart linux disable: remove unit failed: %v", err)
		return err
	}
	if err := runSystemctl("--user", "daemon-reload"); err != nil {
		return err
	}

	fmt.Println("autostart disabled")
	return nil
}

func statusAutostart() error {
	status, err := getAutoStartStatus()
	if err != nil {
		log.Printf("autostart linux status: err=%v", err)
		return err
	}
	log.Printf("autostart linux status: status=%s enabled=%v unit=%s", status.Status, status.Enabled, status.UnitPath)
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
	log.Printf("autostart linux query: command=%q output=%s err=%v", strings.Join(cmd.Args, " "), status, err)
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
	log.Printf("autostart linux systemctl: command=%q output=%s err=%v", strings.Join(cmd.Args, " "), strings.TrimSpace(string(output)), err)
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			return err
		}
		return fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}
