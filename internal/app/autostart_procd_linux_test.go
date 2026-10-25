//go:build linux

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// withProcdSandbox points the procd backend at a temporary directory and
// records every action it would hand to the real init system, so the tests
// never touch /etc or start a service.
func withProcdSandbox(t *testing.T) (dir string, actions *[]string) {
	t.Helper()
	dir = t.TempDir()
	t.Chdir(dir)

	oldScript, oldRCDir, oldEUID, oldRun := procdScriptPath, procdRCDir, procdEUID, procdRunAction
	t.Cleanup(func() {
		procdScriptPath, procdRCDir, procdEUID, procdRunAction = oldScript, oldRCDir, oldEUID, oldRun
	})

	procdScriptPath = filepath.Join(dir, "init.d", procdServiceName)
	procdRCDir = filepath.Join(dir, "rc.d")
	procdEUID = func() int { return 0 }
	recorded := []string{}
	actions = &recorded
	procdRunAction = func(action string) error {
		recorded = append(recorded, action)
		return nil
	}
	return dir, actions
}

func TestDetectInitSystem(t *testing.T) {
	present := map[string]bool{}
	stat := func(path string) (os.FileInfo, error) {
		if present[path] {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	lookPath := func(string) (string, error) {
		if present["systemctl"] {
			return "/usr/bin/systemctl", nil
		}
		return "", exec.ErrNotFound
	}

	if got := detectInitSystem(stat, lookPath); got != initUnknown {
		t.Fatalf("bare host = %v, want unknown", got)
	}

	present["systemctl"] = true
	if got := detectInitSystem(stat, lookPath); got != initSystemd {
		t.Fatalf("systemctl host = %v, want systemd", got)
	}

	// OpenWrt images can ship stray systemd binaries; rc.common + procd wins.
	present["/etc/rc.common"] = true
	present["/sbin/procd"] = true
	if got := detectInitSystem(stat, lookPath); got != initProcd {
		t.Fatalf("openwrt host = %v, want procd", got)
	}

	// rc.common alone (a Debian leftover) is not OpenWrt.
	delete(present, "/sbin/procd")
	if got := detectInitSystem(stat, lookPath); got != initSystemd {
		t.Fatalf("rc.common without procd = %v, want systemd", got)
	}

	present["/run/systemd/system"] = true
	delete(present, "systemctl")
	if got := detectInitSystem(stat, lookPath); got != initSystemd {
		t.Fatalf("running systemd = %v, want systemd", got)
	}
}

func TestProcdScriptContent(t *testing.T) {
	opts := DefaultRunOptions()
	opts.Addr = "0.0.0.0:3719"
	opts.PIDFile = "data/my pid.pid"
	script := procdScriptContent("/opt/ZLT files/$app%", "/opt/ZLT files/zlt's", opts)

	for _, want := range []string{
		"#!/bin/sh /etc/rc.common",
		"START=99",
		"STOP=10",
		"USE_PROCD=1",
		`procd_set_param command '/opt/ZLT files/zlt'\''s' '--workdir' '/opt/ZLT files/$app%' 'run' '--listen' '0.0.0.0:3719' '--pid-file' 'data/my pid.pid'`,
		"procd_set_param respawn 3600 2 0",
		"procd_close_instance",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}

	// Without a pid file the daemon uses its default; no stray empty argument.
	plain := procdScriptContent("/opt/zlt", "/opt/zlt/zlt", DefaultRunOptions())
	if strings.Contains(plain, "--pid-file") {
		t.Fatalf("default options must not add --pid-file:\n%s", plain)
	}
}

func TestProcdScriptSyntax(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh unavailable")
	}
	script := procdScriptContent("/opt/zlt", "/opt/zlt/zlt", DefaultRunOptions())
	path := filepath.Join(t.TempDir(), "zhulingtai")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(sh, "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("invalid init script: %v\n%s", err, out)
	}
}

func TestProcdEnablePreservesAndUpdates(t *testing.T) {
	_, actions := withProcdSandbox(t)

	opts := DefaultRunOptions()
	opts.Addr = "0.0.0.0:3719"
	if err := enableProcdAutostart(opts, false); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(procdScriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `'--listen' '0.0.0.0:3719'`) {
		t.Fatalf("custom listen lost:\n%s", content)
	}
	info, err := os.Stat(procdScriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("script mode = %v, want 0755", info.Mode().Perm())
	}
	if got := strings.Join(*actions, ","); got != "enable,start" {
		t.Fatalf("enable actions = %q, want enable,start (fresh service starts once)", got)
	}

	manual := strings.ReplaceAll(string(content), "0.0.0.0:3719", "0.0.0.0:4000") + "\n# manual edit\n"
	if err := os.WriteFile(procdScriptPath, []byte(manual), 0o755); err != nil {
		t.Fatal(err)
	}
	*actions = nil
	if err := enableProcdAutostart(DefaultRunOptions(), false); err != nil {
		t.Fatal(err)
	}
	content, _ = os.ReadFile(procdScriptPath)
	if string(content) != manual {
		t.Fatalf("re-enable overwrote manual script:\n%s", content)
	}
	if got := strings.Join(*actions, ","); got != "enable,start" {
		t.Fatalf("re-enable actions = %q, want enable,start", got)
	}
}

func TestProcdEnableSkipsStartWhenAlreadyRunning(t *testing.T) {
	_, actions := withProcdSandbox(t)

	if err := os.MkdirAll(filepath.Dir(defaultPIDFile()), 0o755); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writePIDFile(defaultPIDFile(), pidFilePayload{PID: os.Getpid(), Exe: exe}); err != nil {
		t.Fatal(err)
	}

	if err := enableProcdAutostart(DefaultRunOptions(), false); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*actions, ","); got != "enable" {
		t.Fatalf("actions = %q, want only enable when an instance already owns the lock", got)
	}
}

func TestProcdDisable(t *testing.T) {
	_, actions := withProcdSandbox(t)
	if err := os.MkdirAll(filepath.Dir(procdScriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(procdScriptPath, []byte("#!/bin/sh /etc/rc.common\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := disableProcdAutostart(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*actions, ","); got != "stop,disable" {
		t.Fatalf("disable actions = %q, want stop,disable", got)
	}
	if _, err := os.Stat(procdScriptPath); !os.IsNotExist(err) {
		t.Fatalf("script not removed: %v", err)
	}

	if err := disableProcdAutostart(); err != nil {
		t.Fatalf("disable on missing script: %v", err)
	}
}

func TestProcdStatus(t *testing.T) {
	withProcdSandbox(t)

	status, err := procdAutostartStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Supported || status.Enabled || status.Status != "disabled" {
		t.Fatalf("missing script status = %+v", status)
	}

	if err := os.MkdirAll(filepath.Dir(procdScriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(procdScriptPath, []byte("#!/bin/sh /etc/rc.common\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	status, err = procdAutostartStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.Status != "disabled" {
		t.Fatalf("script without rc.d link = %+v, want disabled", status)
	}

	if err := os.MkdirAll(procdRCDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(procdRCDir, "S99"+procdServiceName), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	status, err = procdAutostartStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.Status != "enabled" {
		t.Fatalf("enabled status = %+v", status)
	}
	if status.UnitPath != procdScriptPath {
		t.Fatalf("unit path = %q, want %q", status.UnitPath, procdScriptPath)
	}
}

func TestProcdRequiresRoot(t *testing.T) {
	withProcdSandbox(t)
	procdEUID = func() int { return 1000 }

	err := enableProcdAutostart(DefaultRunOptions(), false)
	if err == nil || !strings.Contains(err.Error(), "requires root") {
		t.Fatalf("enable as non-root = %v, want permission error", err)
	}
	if _, statErr := os.Stat(procdScriptPath); !os.IsNotExist(statErr) {
		t.Fatalf("non-root enable wrote a script: %v", statErr)
	}

	status, err := procdAutostartStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Supported || !strings.Contains(status.Message, "sudo") {
		t.Fatalf("non-root status hint = %+v", status)
	}
}
