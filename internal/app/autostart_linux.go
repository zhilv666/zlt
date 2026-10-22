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

// autostartScope selects which systemd instance manages the autostart unit.
// Root (run directly or through sudo) gets a system-level unit; regular users
// get a user-level unit, because `systemctl --user` only works inside the
// caller's own user session.
type autostartScope int

const (
	scopeUser autostartScope = iota
	scopeSystem
)

func autostartScopeFor(euid int) autostartScope {
	if euid == 0 {
		return scopeSystem
	}
	return scopeUser
}

func currentAutostartScope() autostartScope {
	return autostartScopeFor(os.Geteuid())
}

func (s autostartScope) label() string {
	if s == scopeSystem {
		return "system"
	}
	return "user"
}

// wantedBy is the [Install] target that starts the unit at boot. The system
// manager reaches multi-user.target; the user manager reaches default.target
// once the user session (or their lingering manager) starts.
func (s autostartScope) wantedBy() string {
	if s == scopeSystem {
		return "multi-user.target"
	}
	return "default.target"
}

func (s autostartScope) systemctlArgs(args ...string) []string {
	if s == scopeUser {
		return append([]string{"--user"}, args...)
	}
	return args
}

func systemdUnitPath(scope autostartScope) (string, error) {
	if scope == scopeSystem {
		return filepath.Join("/etc", "systemd", "system", systemdServiceName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", systemdServiceName), nil
}

func systemdUnitContent(scope autostartScope, workdir, exe string, opts RunOptions) string {
	// The executable token has no environment expansion; later arguments do.
	command := systemdQuote(exe, false) + " run --listen " + systemdQuote(opts.Addr, true)
	if opts.PIDFile != "" {
		command += " --pid-file " + systemdQuote(opts.PIDFile, true)
	}
	return strings.TrimSpace(fmt.Sprintf(`
[Unit]
Description=驻令台
After=network.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=always
RestartSec=2

[Install]
WantedBy=%s
`, strings.ReplaceAll(workdir, "%", "%%"), command, scope.wantedBy())) + "\n"
}

// systemd parses its own C-style quoting, specifiers and environment expansion.
// Literal paths must survive all three; shell quoting alone is insufficient.
func systemdQuote(value string, command bool) string {
	value = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`, "%", "%%").Replace(value)
	if command {
		value = strings.ReplaceAll(value, "$", "$$")
	}
	return `"` + value + `"`
}

func writeSystemdUnit(path, content string, update bool) error {
	if update {
		return os.WriteFile(path, []byte(content), 0o644)
	}
	// Re-enabling an existing unit must preserve manual ExecStart changes and
	// other service directives. Explicit CLI options request regeneration.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = file.WriteString(content)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func enableAutostart() error {
	return enableAutostartWithOptions(DefaultRunOptions(), false)
}

func enableAutostartWithOptions(opts RunOptions, update bool) error {
	return enableAutostartInScope(currentAutostartScope(), opts, update)
}

func enableAutostartInScope(scope autostartScope, opts RunOptions, update bool) error {

	exe, err := os.Executable()
	if err != nil {
		autostartLog().Error("enable: resolve executable failed", "err", err)
		return err
	}

	unitPath, err := systemdUnitPath(scope)
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
	if strings.ContainsAny(workdir+exe+opts.Addr+opts.PIDFile, "\r\n\x00") {
		return fmt.Errorf("autostart paths and arguments must not contain newlines or NUL")
	}

	if err := writeSystemdUnit(unitPath, systemdUnitContent(scope, workdir, exe, opts), update); err != nil {
		autostartLog().Error("enable: write unit failed", "err", err)
		return err
	}
	autostartLog().Info("enabled", "scope", scope.label(), "unit", unitPath)

	if err := runSystemctl(scope, "daemon-reload"); err != nil {
		return err
	}
	if err := runSystemctl(scope, "enable", "--now", systemdServiceName); err != nil {
		return err
	}

	fmt.Printf("autostart enabled (%s): %s\n", scope.label(), unitPath)
	if update {
		fmt.Printf("to apply changes to an already running service: systemctl %s\n", strings.Join(scope.systemctlArgs("restart", systemdServiceName), " "))
	}
	return nil
}

func disableAutostart() error {
	scope := currentAutostartScope()

	unitPath, err := systemdUnitPath(scope)
	if err != nil {
		autostartLog().Error("disable: resolve unit path failed", "err", err)
		return err
	}

	_ = runSystemctl(scope, "disable", "--now", systemdServiceName)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		autostartLog().Error("disable: remove unit failed", "err", err)
		return err
	}
	if err := runSystemctl(scope, "daemon-reload"); err != nil {
		return err
	}

	fmt.Printf("autostart disabled (%s)\n", scope.label())
	return nil
}

func statusAutostart() error {
	status, err := getAutoStartStatus()
	if err != nil {
		autostartLog().Error("status query failed", "err", err)
		return err
	}
	autostartLog().Debug("status", "status", status.Status, "enabled", status.Enabled, "unit", status.UnitPath)
	fmt.Printf("autostart: %s (%s)\n", status.Status, currentAutostartScope().label())
	return nil
}

func getAutoStartStatus() (AutoStartStatus, error) {
	scope := currentAutostartScope()

	unitPath, err := systemdUnitPath(scope)
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

	cmd := exec.Command("systemctl", scope.systemctlArgs("is-enabled", systemdServiceName)...)
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

func runSystemctl(scope autostartScope, args ...string) error {
	cmd := exec.Command("systemctl", scope.systemctlArgs(args...)...)
	output, err := cmd.CombinedOutput()
	autostartLog().Debug("systemctl", "command", strings.Join(cmd.Args, " "), "output", strings.TrimSpace(string(output)), "err", err)
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			return err
		}
		return fmt.Errorf("systemctl %s: %s", strings.Join(cmd.Args[1:], " "), msg)
	}
	return nil
}
