package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"zhulingtai/internal/task"
)

type testScheduleManager struct {
	schedules  []task.Schedule
	upserted   []task.Schedule
	deleted    []string
	enabled    map[string]bool
	runResult  task.ScheduleRunResult
	runErr     error
	upsertErr  error
	nextRun    time.Time
	hasNextRun bool
}

func (m *testScheduleManager) ListSchedules() []task.Schedule { return m.schedules }

func (m *testScheduleManager) UpsertSchedule(sch task.Schedule) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.upserted = append(m.upserted, sch)
	return nil
}

func (m *testScheduleManager) DeleteSchedule(id string) error {
	m.deleted = append(m.deleted, id)
	return nil
}

func (m *testScheduleManager) SetScheduleEnabled(id string, enabled bool) error {
	if m.enabled == nil {
		m.enabled = map[string]bool{}
	}
	m.enabled[id] = enabled
	return nil
}

func (m *testScheduleManager) RunScheduleNow(id string) (task.ScheduleRunResult, error) {
	return m.runResult, m.runErr
}

func (m *testScheduleManager) ScheduleNextRun(id string) (time.Time, bool) {
	return m.nextRun, m.hasNextRun
}

func newScheduleTestServer(m *testScheduleManager) *Server {
	return NewServer(
		testRuntime{tasks: []task.Config{{ID: "demo", Name: "Demo", Program: "demo.exe"}}},
		testManager{},
		nil,
		m,
		nil,
	)
}

func TestHandleSchedulesListJoinsTaskAndNextRun(t *testing.T) {
	next := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	manager := &testScheduleManager{
		schedules: []task.Schedule{
			{ID: "sch-1", TaskID: "demo", Name: "morning", CronExpr: "0 8 * * 1-5", Action: task.ScheduleActionStart, Enabled: true},
			{ID: "sch-2", TaskID: "ghost", Name: "orphan", CronExpr: "0 9 * * *", Action: task.ScheduleActionStop},
		},
		nextRun:    next,
		hasNextRun: true,
	}
	server := newScheduleTestServer(manager)

	req := httptest.NewRequest(http.MethodGet, "/api/schedules", nil)
	rec := httptest.NewRecorder()
	server.handleSchedules(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Code int            `json:"code"`
		Data []scheduleItem `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Data))
	}
	first := resp.Data[0]
	if !first.TaskExists || first.TaskName != "Demo" {
		t.Fatalf("task join failed: %+v", first)
	}
	if first.NextRunAt != next.Format(time.RFC3339) {
		t.Fatalf("unexpected next_run_at: %q", first.NextRunAt)
	}
	if resp.Data[1].TaskExists || resp.Data[1].TaskName != "" {
		t.Fatalf("orphan should not resolve a task: %+v", resp.Data[1])
	}
}

func TestHandleSchedulesCreate(t *testing.T) {
	manager := &testScheduleManager{}
	server := newScheduleTestServer(manager)

	body := `{"task_id":"demo","name":"n","cron_expr":"*/5 * * * *","action":"start","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/schedules", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleSchedules(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(manager.upserted) != 1 || manager.upserted[0].TaskID != "demo" {
		t.Fatalf("upsert not called: %+v", manager.upserted)
	}
}

func TestHandleSchedulesCreateRejectsInvalid(t *testing.T) {
	manager := &testScheduleManager{upsertErr: errors.New("invalid cron expression")}
	server := newScheduleTestServer(manager)

	req := httptest.NewRequest(http.MethodPost, "/api/schedules", strings.NewReader(`{"task_id":"demo"}`))
	rec := httptest.NewRecorder()
	server.handleSchedules(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/schedules", strings.NewReader(`not json`))
	rec = httptest.NewRecorder()
	server.handleSchedules(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json, got %d", rec.Code)
	}
}

func TestHandleScheduleActionPutSetsIDFromPath(t *testing.T) {
	manager := &testScheduleManager{}
	server := newScheduleTestServer(manager)

	body := `{"id":"spoofed","task_id":"demo","cron_expr":"*/5 * * * *","action":"stop"}`
	req := httptest.NewRequest(http.MethodPut, "/api/schedules/sch-real", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.handleScheduleAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(manager.upserted) != 1 || manager.upserted[0].ID != "sch-real" {
		t.Fatalf("path id not enforced: %+v", manager.upserted)
	}
}

func TestHandleScheduleActionDeleteEnableDisableRun(t *testing.T) {
	manager := &testScheduleManager{
		runResult: task.ScheduleRunResult{Status: task.ScheduleRunSkipped, Detail: "already_running"},
	}
	server := newScheduleTestServer(manager)

	req := httptest.NewRequest(http.MethodDelete, "/api/schedules/sch-1", nil)
	rec := httptest.NewRecorder()
	server.handleScheduleAction(rec, req)
	if rec.Code != http.StatusOK || len(manager.deleted) != 1 || manager.deleted[0] != "sch-1" {
		t.Fatalf("delete failed: %d %+v", rec.Code, manager.deleted)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/schedules/sch-1/enable", nil)
	rec = httptest.NewRecorder()
	server.handleScheduleAction(rec, req)
	if rec.Code != http.StatusOK || !manager.enabled["sch-1"] {
		t.Fatalf("enable failed: %d %+v", rec.Code, manager.enabled)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/schedules/sch-1/disable", nil)
	rec = httptest.NewRecorder()
	server.handleScheduleAction(rec, req)
	if rec.Code != http.StatusOK || manager.enabled["sch-1"] {
		t.Fatalf("disable failed: %d %+v", rec.Code, manager.enabled)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/schedules/sch-1/run", nil)
	rec = httptest.NewRecorder()
	server.handleScheduleAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("run failed: %d", rec.Code)
	}
	var resp struct {
		Data task.ScheduleRunResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode run response: %v", err)
	}
	if resp.Data.Status != task.ScheduleRunSkipped || resp.Data.Detail != "already_running" {
		t.Fatalf("unexpected run payload: %+v", resp.Data)
	}
}

func TestHandleScheduleMethodGuards(t *testing.T) {
	server := newScheduleTestServer(&testScheduleManager{})

	req := httptest.NewRequest(http.MethodPut, "/api/schedules", nil)
	rec := httptest.NewRecorder()
	server.handleSchedules(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 on collection PUT, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/schedules/sch-1/run", nil)
	rec = httptest.NewRecorder()
	server.handleScheduleAction(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 on GET run, got %d", rec.Code)
	}
}

func TestHandleSchedulesUnavailableWithoutManager(t *testing.T) {
	server := NewServer(testRuntime{}, testManager{}, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/schedules", nil)
	rec := httptest.NewRecorder()
	server.handleSchedules(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without schedule manager, got %d", rec.Code)
	}
}
