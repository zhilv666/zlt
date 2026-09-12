//go:build linux

package app

import (
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
	system := systemdUnitContent(scopeSystem, "/opt/zlt", "/opt/zlt/zlt")
	for _, want := range []string{
		"WorkingDirectory=/opt/zlt",
		"ExecStart=/opt/zlt/zlt run",
		"Restart=always",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("system unit missing %q:\n%s", want, system)
		}
	}

	user := systemdUnitContent(scopeUser, "/home/u/zlt", "/home/u/zlt/zlt")
	if !strings.Contains(user, "WantedBy=default.target") {
		t.Fatalf("user unit missing default.target:\n%s", user)
	}
	if strings.Contains(user, "multi-user.target") {
		t.Fatalf("user unit should not target multi-user.target:\n%s", user)
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
