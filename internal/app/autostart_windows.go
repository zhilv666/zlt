//go:build windows

package app

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const windowsRunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
const windowsAutostartValue = "驻令台"

func enableAutostart() error {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("autostart windows enable: resolve executable failed: %v", err)
		return err
	}

	command := fmt.Sprintf("%q run", exe)
	cmd := exec.Command("reg", "add", windowsRunKey, "/v", windowsAutostartValue, "/t", "REG_SZ", "/d", command, "/f")
	output, err := cmd.CombinedOutput()
	log.Printf("autostart windows enable: command=%q output=%s err=%v", strings.Join(cmd.Args, " "), strings.TrimSpace(string(output)), err)
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			return err
		}
		return fmt.Errorf("reg add autostart: %s", msg)
	}
	return nil
}

func disableAutostart() error {
	cmd := exec.Command("reg", "delete", windowsRunKey, "/v", windowsAutostartValue, "/f")
	output, err := cmd.CombinedOutput()
	log.Printf("autostart windows disable: command=%q output=%s err=%v", strings.Join(cmd.Args, " "), strings.TrimSpace(string(output)), err)
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			return err
		}
		if strings.Contains(strings.ToLower(msg), "unable to find") {
			return nil
		}
		return fmt.Errorf("reg delete autostart: %s", msg)
	}
	return nil
}

func statusAutostart() error {
	status, err := getAutoStartStatus()
	if err != nil {
		log.Printf("autostart windows status: err=%v", err)
		return err
	}
	log.Printf("autostart windows status: status=%s enabled=%v unit=%s", status.Status, status.Enabled, status.UnitPath)
	fmt.Printf("autostart: %s\n", status.Status)
	return nil
}

func getAutoStartStatus() (AutoStartStatus, error) {
	cmd := exec.Command("reg", "query", windowsRunKey, "/v", windowsAutostartValue)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	log.Printf("autostart windows query: command=%q output=%s err=%v", strings.Join(cmd.Args, " "), text, err)
	if err != nil {
		if strings.Contains(strings.ToLower(text), "unable to find") {
			return AutoStartStatus{
				Supported: true,
				Enabled:   false,
				Status:    "disabled",
				UnitPath:  filepath.Join("HKCU", "Software", "Microsoft", "Windows", "CurrentVersion", "Run"),
			}, nil
		}
		if text == "" {
			return AutoStartStatus{}, err
		}
		return AutoStartStatus{}, fmt.Errorf("reg query autostart: %s", text)
	}

	return AutoStartStatus{
		Supported: true,
		Enabled:   true,
		Status:    "enabled",
		UnitPath:  filepath.Join("HKCU", "Software", "Microsoft", "Windows", "CurrentVersion", "Run"),
	}, nil
}
