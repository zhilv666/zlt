package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	rootassets "tray"
	"tray/internal/buildinfo"
	"tray/internal/process"
	"tray/internal/task"
)

type Runtime interface {
	ListTasks() []task.Config
	UpsertTask(task.Config) error
	DeleteTask(string) error
	RestartTask(string) error
	ExportTasks() []task.Config
	ReplaceTasks([]task.Config) error
}

type ProcessManager interface {
	States() []process.RuntimeState
	State(string) (process.RuntimeState, bool)
	Start(string) error
	Stop(string) error
	ClearLogs(string) error
}

type Server struct {
	runtime Runtime
	manager ProcessManager
}

type response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

type taskItem struct {
	Task   task.Config          `json:"task"`
	Status process.RuntimeState `json:"status"`
}

func NewServer(runtime Runtime, manager ProcessManager) *Server {
	return &Server{
		runtime: runtime,
		manager: manager,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/build-info", s.handleBuildInfo)
	mux.HandleFunc("/api/events/tasks", s.handleTaskEvents)
	mux.HandleFunc("/api/tasks", s.handleTasks)
	mux.HandleFunc("/api/tasks/", s.handleTaskAction)
	mux.HandleFunc("/api/tasks-export", s.handleTasksExport)
	mux.HandleFunc("/api/tasks-import", s.handleTasksImport)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(rootassets.IndexHTML))
}

func (s *Server) handleBuildInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
		return
	}

	writeJSON(w, http.StatusOK, response{
		Code: 0,
		Msg:  "ok",
		Data: buildinfo.Current(),
	})
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "ok", Data: s.buildTaskItems()})
	case http.MethodPost:
		var payload task.Config
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: "invalid json"})
			return
		}
		if err := s.runtime.UpsertTask(payload); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "saved"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
	}
}

func (s *Server) handleTasksExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="tasks-export.json"`)
	_ = json.NewEncoder(w).Encode(s.runtime.ExportTasks())
}

func (s *Server) handleTasksImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
		return
	}

	var payload []task.Config
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: "invalid json"})
		return
	}

	if err := s.runtime.ReplaceTasks(payload); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, response{Code: 0, Msg: "imported"})
}

func (s *Server) handleTaskEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastPayload []byte
	for {
		payload, err := json.Marshal(s.buildTaskItems())
		if err != nil {
			return
		}
		if !bytes.Equal(payload, lastPayload) {
			if err := writeSSEEvent(w, "tasks", payload); err != nil {
				return
			}
			flusher.Flush()
			lastPayload = payload
		}

		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) handleTaskAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	taskID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPut:
			var payload task.Config
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: "invalid json"})
				return
			}
			payload.ID = taskID
			if err := s.runtime.UpsertTask(payload); err != nil {
				writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, response{Code: 0, Msg: "updated"})
		case http.MethodDelete:
			if err := s.runtime.DeleteTask(taskID); err != nil {
				writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, response{Code: 0, Msg: "deleted"})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
		}
		return
	}

	action := parts[1]
	switch action {
	case "start":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		if err := s.manager.Start(taskID); err != nil {
			writeTaskActionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "started"})
	case "stop":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		if err := s.manager.Stop(taskID); err != nil {
			writeTaskActionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "stopped"})
	case "restart":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		if err := s.runtime.RestartTask(taskID); err != nil {
			writeTaskActionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "restarted"})
	case "status":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		state, ok := s.manager.State(taskID)
		if !ok {
			writeJSON(w, http.StatusNotFound, response{Code: 1, Msg: "task not found"})
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "ok", Data: state})
	case "logs":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		logType := r.URL.Query().Get("type")
		if logType == "" {
			logType = "stdout"
		}
		if logType != "stdout" && logType != "stderr" {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: "invalid log type"})
			return
		}
		tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
		if tail <= 0 {
			tail = 200
		}
		if tail > 2000 {
			tail = 2000
		}

		data, err := readLogTail(filepath.Join("data", "logs", taskID, logType+".log"), tail)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "ok", Data: map[string]string{"content": data}})
	case "logs-stream":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		s.handleLogStream(w, r, taskID)
	case "download-logs":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		logType := r.URL.Query().Get("type")
		if logType == "" {
			logType = "stdout"
		}
		if logType != "stdout" && logType != "stderr" {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: "invalid log type"})
			return
		}
		logPath := filepath.Join("data", "logs", taskID, logType+".log")
		data, err := os.ReadFile(logPath)
		if err != nil && !os.IsNotExist(err) {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+taskID+"-"+logType+`.log"`)
		_, _ = w.Write(data)
	case "clear-logs":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		if err := s.manager.ClearLogs(taskID); err != nil {
			writeTaskActionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "logs cleared"})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request, taskID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	logType := r.URL.Query().Get("type")
	if logType == "" {
		logType = "stdout"
	}
	if logType != "stdout" && logType != "stderr" {
		writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: "invalid log type"})
		return
	}

	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 {
		tail = 400
	}
	if tail > 2000 {
		tail = 2000
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	logPath := filepath.Join("data", "logs", taskID, logType+".log")
	var lastPayload []byte
	for {
		content, err := readLogTail(logPath, tail)
		if err != nil {
			return
		}

		payload, err := json.Marshal(map[string]string{
			"task_id": taskID,
			"type":    logType,
			"content": content,
		})
		if err != nil {
			return
		}

		if !bytes.Equal(payload, lastPayload) {
			if err := writeSSEEvent(w, "logs", payload); err != nil {
				return
			}
			flusher.Flush()
			lastPayload = payload
		}

		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) buildTaskItems() []taskItem {
	tasks := s.runtime.ListTasks()
	states := s.manager.States()
	stateMap := make(map[string]process.RuntimeState, len(states))
	for _, st := range states {
		stateMap[st.TaskID] = st
	}

	items := make([]taskItem, 0, len(tasks))
	for _, cfg := range tasks {
		items = append(items, taskItem{
			Task:   cfg,
			Status: stateMap[cfg.ID],
		})
	}
	return items
}

func readLogTail(path string, tail int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return strings.Join(lines, "\n"), nil
}

func writeSSEEvent(w http.ResponseWriter, event string, payload []byte) error {
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeTaskActionError(w http.ResponseWriter, err error) {
	if errors.Is(err, process.ErrTaskNotFound) {
		writeJSON(w, http.StatusNotFound, response{Code: 1, Msg: err.Error()})
		return
	}
	writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
}
