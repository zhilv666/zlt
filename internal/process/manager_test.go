package process

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tray/internal/task"
)

func TestPerformHealthCheckAcceptsHealthyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := performHealthCheck(server.Client(), server.URL); err != nil {
		t.Fatalf("expected healthy response, got %v", err)
	}
}

func TestPerformHealthCheckRejectsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if err := performHealthCheck(server.Client(), server.URL); err == nil {
		t.Fatalf("expected health check error")
	}
}

func TestPrepareRestartLockedHonorsMaxRestartCount(t *testing.T) {
	manager := &Manager{}
	proc := &managedProcess{
		task: task.Config{
			ID:              "demo",
			RestartOnCrash:  true,
			RestartDelaySec: 7,
			MaxRestartCount: 2,
		},
		state: RuntimeState{
			TaskID: "demo",
		},
		restartCount: 2,
	}

	taskID, delaySec, shouldRestart := manager.prepareRestartLocked(proc)
	if taskID != "demo" || delaySec != 7 {
		t.Fatalf("unexpected restart plan: task=%q delay=%d", taskID, delaySec)
	}
	if shouldRestart {
		t.Fatalf("expected restart to stop after reaching max count")
	}
}
