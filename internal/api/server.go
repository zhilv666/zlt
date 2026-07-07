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

	rootassets "zhulingtai"
	"zhulingtai/internal/buildinfo"
	"zhulingtai/internal/process"
	"zhulingtai/internal/task"
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
	runtime   Runtime
	manager   ProcessManager
	autostart AutoStartManager
}

type AutoStartManager interface {
	Status() (AutoStartStatus, error)
	Enable() error
	Disable() error
}

type AutoStartStatus struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Status    string `json:"status"`
	UnitPath  string `json:"unit_path,omitempty"`
	Message   string `json:"message,omitempty"`
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

func NewServer(runtime Runtime, manager ProcessManager, autostart AutoStartManager) *Server {
	return &Server{
		runtime:   runtime,
		manager:   manager,
		autostart: autostart,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/build-info", s.handleBuildInfo)
	mux.HandleFunc("/api/system-log", s.handleSystemLog)
	mux.HandleFunc("/api/system-log-stream", s.handleSystemLogStream)
	mux.HandleFunc("/api/system-log-download", s.handleSystemLogDownload)
	mux.HandleFunc("/api/system-log-clear", s.handleSystemLogClear)
	mux.HandleFunc("/api/autostart", s.handleAutoStart)
	mux.HandleFunc("/api/autostart/", s.handleAutoStart)
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

func (s *Server) handleSystemLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
		return
	}

	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 {
		tail = 200
	}
	if tail > 2000 {
		tail = 2000
	}

	data, err := readLogTail(systemLogPath(), tail)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Code: 0, Msg: "ok", Data: map[string]string{"content": data}})
}

func (s *Server) handleSystemLogDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
		return
	}

	data, err := os.ReadFile(systemLogPath())
	if err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="app.log"`)
	_, _ = w.Write([]byte(decodeLogText(data)))
}

func (s *Server) handleSystemLogClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
		return
	}
	if err := os.WriteFile(systemLogPath(), []byte{}, 0o644); err != nil {
		writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{Code: 0, Msg: "logs cleared"})
}

func (s *Server) handleSystemLogStream(w http.ResponseWriter, r *http.Request) {
	s.handleGenericLogStream(w, r, systemLogPath(), "system")
}

func (s *Server) handleAutoStart(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/autostart")
	switch path {
	case "":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		if s.autostart == nil {
			writeJSON(w, http.StatusOK, response{Code: 0, Msg: "ok", Data: AutoStartStatus{Supported: false, Status: "unsupported"}})
			return
		}
		status, err := s.autostart.Status()
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "ok", Data: status})
	case "/enable":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		if s.autostart == nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: "autostart is not available"})
			return
		}
		if err := s.autostart.Enable(); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		status, _ := s.autostart.Status()
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "enabled", Data: status})
	case "/disable":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		if s.autostart == nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: "autostart is not available"})
			return
		}
		if err := s.autostart.Disable(); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		status, _ := s.autostart.Status()
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "disabled", Data: status})
	default:
		http.NotFound(w, r)
	}
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
		tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
		if tail <= 0 {
			tail = 200
		}
		if tail > 2000 {
			tail = 2000
		}

		data, err := readTaskLogTail(taskID, tail)
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
		s.handleTaskLogStream(w, r, taskID)
	case "download-logs":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		data, err := readTaskLog(taskID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+taskID+`.log"`)
		_, _ = w.Write([]byte(data))
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

func (s *Server) handleTaskLogStream(w http.ResponseWriter, r *http.Request, taskID string) {
	s.handleGenericLogStream(w, r, "", taskID)
}

func (s *Server) handleGenericLogStream(w http.ResponseWriter, r *http.Request, path string, sourceID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
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

	var lastPayload []byte
	for {
		var (
			content string
			err     error
		)
		if path != "" {
			content, err = readLogTail(path, tail)
		} else {
			content, err = readTaskLogTail(sourceID, tail)
		}
		if err != nil {
			return
		}

		payload, err := json.Marshal(map[string]string{
			"task_id": sourceID,
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

	lines := strings.Split(decodeLogText(data), "\n")
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return strings.Join(lines, "\n"), nil
}

func taskLogPath(taskID string) string {
	return filepath.Join("data", "logs", taskID, "app.log")
}

func systemLogPath() string {
	return filepath.Join("data", "app.log")
}

func taskLegacyStdoutPath(taskID string) string {
	return filepath.Join("data", "logs", taskID, "stdout.log")
}

func taskLegacyStderrPath(taskID string) string {
	return filepath.Join("data", "logs", taskID, "stderr.log")
}

func readTaskLog(taskID string) (string, error) {
	combinedPath := taskLogPath(taskID)
	combinedData, err := os.ReadFile(combinedPath)
	if err == nil {
		return decodeLogText(combinedData), nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}

	stdoutData, stdoutErr := os.ReadFile(taskLegacyStdoutPath(taskID))
	if stdoutErr != nil && !os.IsNotExist(stdoutErr) {
		return "", stdoutErr
	}

	stderrData, stderrErr := os.ReadFile(taskLegacyStderrPath(taskID))
	if stderrErr != nil && !os.IsNotExist(stderrErr) {
		return "", stderrErr
	}

	var builder strings.Builder
	if len(stdoutData) > 0 {
		builder.Write(stdoutData)
	}
	if len(stderrData) > 0 {
		if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "\n") {
			builder.WriteByte('\n')
		}
		builder.Write(stderrData)
	}
	return decodeLogText([]byte(builder.String())), nil
}

func readTaskLogTail(taskID string, tail int) (string, error) {
	content, err := readTaskLog(taskID)
	if err != nil {
		return "", err
	}
	if content == "" {
		return "", nil
	}

	lines := strings.Split(content, "\n")
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
