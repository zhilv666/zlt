package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"zhulingtai/internal/task"
)

// ScheduleManager is the slice of Runtime the schedule endpoints drive.
type ScheduleManager interface {
	ListSchedules() []task.Schedule
	UpsertSchedule(task.Schedule) error
	DeleteSchedule(string) error
	SetScheduleEnabled(string, bool) error
	RunScheduleNow(string) (task.ScheduleRunResult, error)
	ScheduleNextRun(string) (time.Time, bool)
}

type scheduleItem struct {
	Schedule   task.Schedule `json:"schedule"`
	NextRunAt  string        `json:"next_run_at,omitempty"`
	TaskName   string        `json:"task_name,omitempty"`
	TaskExists bool          `json:"task_exists"`
}

func (s *Server) schedulesUnavailable(w http.ResponseWriter) bool {
	if s.schedules == nil {
		writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: "schedules are not available"})
		return true
	}
	return false
}

func (s *Server) handleSchedules(w http.ResponseWriter, r *http.Request) {
	if s.schedulesUnavailable(w) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "ok", Data: s.buildScheduleItems()})
	case http.MethodPost:
		var payload task.Schedule
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: "invalid json"})
			return
		}
		if err := s.schedules.UpsertSchedule(payload); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "saved"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
	}
}

func (s *Server) handleScheduleAction(w http.ResponseWriter, r *http.Request) {
	if s.schedulesUnavailable(w) {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/schedules/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	scheduleID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPut:
			var payload task.Schedule
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: "invalid json"})
				return
			}
			payload.ID = scheduleID
			if err := s.schedules.UpsertSchedule(payload); err != nil {
				writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, response{Code: 0, Msg: "updated"})
		case http.MethodDelete:
			if err := s.schedules.DeleteSchedule(scheduleID); err != nil {
				writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, response{Code: 0, Msg: "deleted"})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
		}
		return
	}

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
		return
	}

	switch parts[1] {
	case "enable":
		if err := s.schedules.SetScheduleEnabled(scheduleID, true); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "enabled"})
	case "disable":
		if err := s.schedules.SetScheduleEnabled(scheduleID, false); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "disabled"})
	case "run":
		result, err := s.schedules.RunScheduleNow(scheduleID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "executed", Data: result})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) buildScheduleItems() []scheduleItem {
	schedules := s.schedules.ListSchedules()

	taskNames := make(map[string]string)
	for _, cfg := range s.runtime.ListTasks() {
		taskNames[cfg.ID] = cfg.Name
	}

	items := make([]scheduleItem, 0, len(schedules))
	for _, sch := range schedules {
		item := scheduleItem{Schedule: sch}
		if name, ok := taskNames[sch.TaskID]; ok {
			item.TaskName = name
			item.TaskExists = true
		}
		if next, ok := s.schedules.ScheduleNextRun(sch.ID); ok {
			item.NextRunAt = next.Format(time.RFC3339)
		}
		items = append(items, item)
	}
	return items
}
