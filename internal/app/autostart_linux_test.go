//go:build linux

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutostartScopeFor(t *testing.T) {
	if got := autostartScopeFor(0); got != scopeSystem {
		t.Fatalf("root should use system scope, got %v", got)
	}
	if got := autostartScopeFor(1000); got != scopeUser {
		t.Fatalf("regular user should use user scope, got %v", got)
	}
}

func TestSystemdUnitPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	userPath, err := systemdUnitPath(scopeUser)
	if err != nil {
		t.Fatalf("user unit path: %v", err)
	}
	wantUser := filepath.Join(home, ".config", "systemd", "user", systemdServiceName)
	if userPath != wantUser {
		t.Fatalf("user unit path = %q, want %q", userPath, wantUser)
	}

	systemPath, err := systemdUnitPath(scopeSystem)
	if err != nil {
		t.Fatalf("system unit path: %v", err)
	}
	if want := filepath.Join("/etc", "systemd", "system", systemdServiceName); systemPath != want {
		t.Fatalf("system unit path = %q, want %q", systemPath, want)
	}
}

func TestSystemdUnitContent(t *testing.T) {
	system := systemdUnitContent(scopeSystem, "/opt/zlt", "/opt/zlt/zlt", DefaultRunOptions())
	for _, want := range []string{
		`WorkingDirectory=/opt/zlt`,
		`ExecStart="/opt/zlt/zlt" run --listen "127.0.0.1:3719"`,
		"Restart=always",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("system unit missing %q:\n%s", want, system)
		}
	}

	user := systemdUnitContent(scopeUser, "/home/u/zlt", "/home/u/zlt/zlt", DefaultRunOptions())
	if !strings.Contains(user, "WantedBy=default.target") {
		t.Fatalf("user unit missing default.target:\n%s", user)
	}
	if strings.Contains(user, "multi-user.target") {
		t.Fatalf("user unit should not target multi-user.target:\n%s", user)
	}
}

func TestSystemdAutostartCustomOptionsAndPreservation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := t.TempDir()
	trace := filepath.Join(bin, "trace")
	t.Setenv("ZLT_SYSTEMCTL_TRACE", trace)
	// The test writes only a temporary user unit; no host services are changed.
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$ZLT_SYSTEMCTL_TRACE\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	opts := DefaultRunOptions()
	opts.Addr = "0.0.0.0:3719"
	opts.PIDFile = "data/custom pid%$.pid"
	if err := enableAutostartInScope(scopeUser, opts, false); err != nil {
		t.Fatal(err)
	}
	unitPath, _ := systemdUnitPath(scopeUser)
	content, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `run --listen "0.0.0.0:3719" --pid-file "data/custom pid%%$$.pid"`) {
		t.Fatalf("custom options lost:\n%s", content)
	}
	manual := strings.ReplaceAll(string(content), "0.0.0.0:3719", "0.0.0.0:4000") + "\n# manual customization\n"
	if err := os.WriteFile(unitPath, []byte(manual), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := enableAutostartInScope(scopeUser, DefaultRunOptions(), false); err != nil {
		t.Fatal(err)
	}
	content, _ = os.ReadFile(unitPath)
	if string(content) != manual {
		t.Fatalf("re-enable overwrote custom unit:\n%s", content)
	}
	opts.Addr = "0.0.0.0:5000"
	if err := enableAutostartInScope(scopeUser, opts, true); err != nil {
		t.Fatal(err)
	}
	content, _ = os.ReadFile(unitPath)
	if !strings.Contains(string(content), `--listen "0.0.0.0:5000"`) {
		t.Fatalf("explicit update not applied:\n%s", content)
	}
	commands, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("--user daemon-reload\n--user enable --now zhulingtai.service\n", 3)
	if string(commands) != want {
		t.Fatalf("unexpected service operations: %q", commands)
	}
}

func TestSystemdUnitEscapesPaths(t *testing.T) {
	if got := systemdQuote("/opt/ZLT files/%app$\\\"\n", true); got != `"/opt/ZLT files/%%app$$\\\"\n"` {
		t.Fatalf("unexpected quoting: %s", got)
	}
	if got := systemdQuote("/opt/$app%", false); got != `"/opt/$app%%"` {
		t.Fatalf("unexpected directory quoting: %s", got)
	}
}

func TestSystemdUnitSyntax(t *testing.T) {
	tool, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Skip("systemd-analyze unavailable")
	}
	dir := t.TempDir()
	dir = filepath.Join(dir, "work dir %$")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "zlt %$ executable")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	unit := filepath.Join(dir, "zlt-syntax-test.service")
	opts := DefaultRunOptions()
	opts.Addr = "0.0.0.0:3719"
	if err := os.WriteFile(unit, []byte(systemdUnitContent(scopeSystem, dir, exe, opts)), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(tool, "verify", unit).CombinedOutput(); err != nil {
		t.Fatalf("invalid systemd unit: %v\n%s", err, out)
	}
}

func TestSystemctlArgs(t *testing.T) {
	user := scopeUser.systemctlArgs("enable", "--now", systemdServiceName)
	wantUser := []string{"--user", "enable", "--now", systemdServiceName}
	if strings.Join(user, " ") != strings.Join(wantUser, " ") {
		t.Fatalf("user systemctl args = %v, want %v", user, wantUser)
	}

	system := scopeSystem.systemctlArgs("enable", "--now", systemdServiceName)
	wantSystem := []string{"enable", "--now", systemdServiceName}
	if strings.Join(system, " ") != strings.Join(wantSystem, " ") {
		t.Fatalf("system systemctl args = %v, want %v", system, wantSystem)
	}
}
