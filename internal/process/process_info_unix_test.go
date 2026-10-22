//go:build linux || darwin

package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"zhulingtai/internal/task"
)

func TestUnixProcessIdentityAndWorkingDirectory(t *testing.T) {
	cfg := regressionTask(t, "hold")
	workdir := filepath.Join(cfg.WorkDir, "work dir\nwith newline")
	if err := os.Mkdir(workdir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg.WorkDir = workdir
	cfg.Args = append(cfg.Args, "--", "path with spaces", "", `quoted"value`, "")
	marker := filepath.Join(t.TempDir(), "started")
	cfg.Env = append(cfg.Env, "ZLT_REGRESSION_MARKER="+marker)
	cmd := exec.Command(cfg.Program, cfg.Args...)
	cmd.Dir = cfg.WorkDir
	cmd.Env = append(os.Environ(), cfg.Env...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	// Start returns before the helper is ready; /proc identity may still be
	// incomplete during exec. Wait for its startup marker before asserting it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(marker)
		if err == nil && string(data) == "start\n" {
			break
		}
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not signal readiness")
		}
		time.Sleep(10 * time.Millisecond)
	}
	exe, argv, ok := readUnixProcessCommand(cmd.Process.Pid)
	if !ok || !sameProcessFile(exe, cfg.Program) || len(argv) == 0 || !reflect.DeepEqual(argv[1:], cfg.Args) {
		t.Fatalf("incorrect process identity: exe=%q argv=%q ok=%v", exe, argv, ok)
	}
	if !processMatchesWorkingDir(cmd.Process.Pid, cfg.WorkDir) {
		t.Fatalf("cwd did not match %q", cfg.WorkDir)
	}
	symlink := filepath.Join(t.TempDir(), "workdir-link")
	if err := os.Symlink(cfg.WorkDir, symlink); err != nil {
		t.Fatal(err)
	}
	if !processMatchesWorkingDir(cmd.Process.Pid, symlink) {
		t.Fatal("equivalent cwd symlink was not recognized")
	}
	wrongWorkdir := cfg
	wrongWorkdir.WorkDir = t.TempDir()
	wrongArgs := cfg
	wrongArgs.Args = append([]string{}, cfg.Args...)
	wrongArgs.Args[len(wrongArgs.Args)-1] = "different"
	wrongProgram := cfg
	wrongProgram.Program = filepath.Join(t.TempDir(), filepath.Base(cfg.Program))
	data, err := os.ReadFile(cfg.Program)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrongProgram.Program, data, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []task.Config{wrongWorkdir, wrongArgs, wrongProgram} {
		if pid, found := findExistingProcess(wrong); found {
			t.Fatalf("adopted pid %d despite mismatched identity: %+v", pid, wrong)
		}
	}
	if pid, found := findExistingProcess(cfg); !found || pid != cmd.Process.Pid {
		t.Fatalf("matching process was not adopted: pid=%d found=%v", pid, found)
	}
}
