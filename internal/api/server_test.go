package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zhulingtai/internal/process"
	"zhulingtai/internal/task"
)

func TestReadLogTailReturnsMissingAsEmpty(t *testing.T) {
	content, err := readLogTail(filepath.Join(t.TempDir(), "missing.log"), 10)
	if err != nil {
		t.Fatalf("read missing log: %v", err)
	}
	if content != "" {
		t.Fatalf("expected empty content, got %q", content)
	}
}

func TestReadLogTailReturnsLastNLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("1\n2\n3\n4\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	// A trailing newline must not cost a line: tail=2 of a 4-line file is "3","4".
	content, err := readLogTail(path, 2)
	if err != nil {
		t.Fatalf("read log tail: %v", err)
	}
	if strings.TrimSpace(content) != "3\n4" {
		t.Fatalf("unexpected log tail %q", content)
	}
}

func TestReadTaskLogFallsBackToLegacyStdoutStderr(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data", "logs", "demo"), 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := os.WriteFile(filepath.Join("data", "logs", "demo", "stdout.log"), []byte("out-1\nout-2\n"), 0o644); err != nil {
		t.Fatalf("write stdout log: %v", err)
	}
	if err := os.WriteFile(filepath.Join("data", "logs", "demo", "stderr.log"), []byte("err-1\n"), 0o644); err != nil {
		t.Fatalf("write stderr log: %v", err)
	}

	content, err := readTaskLog("demo")
	if err != nil {
		t.Fatalf("read task log: %v", err)
	}
	if !strings.Contains(content, "out-1") || !strings.Contains(content, "err-1") {
		t.Fatalf("unexpected merged log content %q", content)
	}
}

type testRuntime struct {
	tasks []task.Config
}

func (r testRuntime) ListTasks() []task.Config       { return r.tasks }
func (testRuntime) UpsertTask(task.Config) error     { return nil }
func (testRuntime) DeleteTask(string) error          { return nil }
func (testRuntime) RestartTask(string) error         { return nil }
func (r testRuntime) ExportTasks() []task.Config     { return r.tasks }
func (testRuntime) ReplaceTasks([]task.Config) error { return nil }

type testManager struct {
	states []process.RuntimeState
}

func (m testManager) States() []process.RuntimeState          { return m.states }
func (testManager) State(string) (process.RuntimeState, bool) { return process.RuntimeState{}, false }
func (testManager) Start(string) error                        { return nil }
func (testManager) Stop(string) error                         { return nil }
func (testManager) ClearLogs(string) error                    { return nil }

type testAutoStart struct {
	status AutoStartStatus
}

func (a testAutoStart) Status() (AutoStartStatus, error) { return a.status, nil }
func (testAutoStart) Enable() error                      { return nil }
func (testAutoStart) Disable() error                     { return nil }

func TestWriteTaskActionErrorUsesNotFoundForMissingTask(t *testing.T) {
	rec := httptest.NewRecorder()
	writeTaskActionError(rec, process.ErrTaskNotFound)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestBuildTaskItemsMatchesStates(t *testing.T) {
	server := NewServer(
		testRuntime{
			tasks: []task.Config{{ID: "openlist", Name: "OpenList", Program: "openlist.exe"}},
		},
		testManager{
			states: []process.RuntimeState{{TaskID: "openlist", Status: process.StatusRunning, PID: 1234}},
		},
		nil,
		nil,
	)

	items := server.buildTaskItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Task.ID != "openlist" || items[0].Status.PID != 1234 || items[0].Status.Status != process.StatusRunning {
		t.Fatalf("unexpected item: %+v", items[0])
	}
}

func TestHandleTaskEventsStreamsInitialSnapshot(t *testing.T) {
	server := httptest.NewServer(NewServer(
		testRuntime{
			tasks: []task.Config{{ID: "demo", Name: "Demo", Program: "demo.exe"}},
		},
		testManager{
			states: []process.RuntimeState{{TaskID: "demo", Status: process.StatusStopped}},
		},
		nil,
		nil,
	).Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/events/tasks")
	if err != nil {
		t.Fatalf("open task stream: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	var eventLine string
	var dataLine string
	for i := 0; i < 6; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream line: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "event: ") {
			eventLine = line
		}
		if strings.HasPrefix(line, "data: ") {
			dataLine = line
			break
		}
	}

	if eventLine != "event: tasks" {
		t.Fatalf("unexpected event line: %q", eventLine)
	}
	if !strings.Contains(dataLine, `"id":"demo"`) {
		t.Fatalf("unexpected data line: %q", dataLine)
	}
}

func TestHandleAutoStartMethodGuard(t *testing.T) {
	server := NewServer(testRuntime{}, testManager{}, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/autostart", nil)
	rec := httptest.NewRecorder()

	server.handleAutoStart(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleAutoStartStatus(t *testing.T) {
	server := NewServer(testRuntime{}, testManager{}, testAutoStart{
		status: AutoStartStatus{Supported: true, Enabled: true, Status: "enabled"},
	}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/autostart", nil)
	rec := httptest.NewRecorder()

	server.handleAutoStart(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data type: %#v", resp.Data)
	}
	if data["status"] != "enabled" {
		t.Fatalf("unexpected autostart status: %#v", data)
	}
}

func TestHandleAutoStartDisabledFallback(t *testing.T) {
	server := NewServer(testRuntime{}, testManager{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/autostart", nil)
	rec := httptest.NewRecorder()

	server.handleAutoStart(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data == nil {
		t.Fatalf("expected autostart data")
	}
}
