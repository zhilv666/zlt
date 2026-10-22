package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestRegressionPIDFileConcurrentAcquisition(t *testing.T) {
	dir := t.TempDir()
	for round := 0; round < 10; round++ {
		path := filepath.Join(dir, fmt.Sprintf("round-%d.pid", round))
		start := make(chan struct{})
		results := make(chan *pidLock, 64)
		var wg sync.WaitGroup
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				lock, err := acquirePIDFile(path, "127.0.0.1:0")
				if err != nil && !errors.Is(err, errAlreadyRunning) {
					t.Errorf("acquire: %v", err)
				}
				results <- lock
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		successes := 0
		for lock := range results {
			if lock != nil {
				successes++
				if err := lock.Release(); err != nil {
					t.Error(err)
				}
			}
		}
		if successes != 1 {
			t.Fatalf("%d concurrent starts acquired the same PID file in round %d", successes, round)
		}
	}
}

func TestRegressionPIDReleaseOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zlt.pid")
	first, err := acquirePIDFile(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	// Same PID and executable, but another ownership token.
	other, err := writePIDFile(path, pidFilePayload{PID: first.PID, Exe: first.Exe})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	current, err := readPIDFile(path)
	if err != nil || current.Token != other.Token {
		t.Fatalf("release removed another owner's payload: %+v %v", current, err)
	}
	if err := other.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := acquirePIDFile(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()
	if err := first.Release(); !errors.Is(err, errAlreadyRunning) {
		t.Fatalf("old owner should not acquire new owner's lock: %v", err)
	}
	if current, err := readPIDFile(path); err != nil || current.Token != second.Token {
		t.Fatalf("old owner removed new lock: %v", err)
	}
}

func TestRegressionPIDLockChild(t *testing.T) {
	if path := os.Getenv("ZLT_PID_LOCK_TEST"); path != "" {
		lock, err := acquirePIDFile(path, "")
		if errors.Is(err, errAlreadyRunning) {
			os.Exit(23)
		}
		if err != nil {
			os.Exit(24)
		}
		_ = lock.Release()
		os.Exit(0)
	}
}

func TestRegressionPIDLockCrossProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zlt.pid")
	lock, err := acquirePIDFile(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	// Remove the payload to ensure the OS lock itself guards startup.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		cmd := exec.Command(mustExecutable(t), "-test.run=^TestRegressionPIDLockChild$")
		cmd.Env = append(os.Environ(), "ZLT_PID_LOCK_TEST="+path)
		err := cmd.Run()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 23 {
			t.Fatalf("child acquired a held lock: %v", err)
		}
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(mustExecutable(t), "-test.run=^TestRegressionPIDLockChild$")
	cmd.Env = append(os.Environ(), "ZLT_PID_LOCK_TEST="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("released lock not available to child: %v %s", err, out)
	}
}
