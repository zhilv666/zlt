package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tray/internal/buildinfo"
	"tray/internal/process"
	"tray/internal/task"
)

type Runtime interface {
	ListTasks() []task.Config
	UpsertTask(task.Config) error
	DeleteTask(string) error
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
	mux.HandleFunc("/api/tasks", s.handleTasks)
	mux.HandleFunc("/api/tasks/", s.handleTaskAction)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
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
		type item struct {
			Task   task.Config          `json:"task"`
			Status process.RuntimeState `json:"status"`
		}

		tasks := s.runtime.ListTasks()
		states := s.manager.States()
		stateMap := make(map[string]process.RuntimeState, len(states))
		for _, st := range states {
			stateMap[st.TaskID] = st
		}

		items := make([]item, 0, len(tasks))
		for _, cfg := range tasks {
			items = append(items, item{
				Task:   cfg,
				Status: stateMap[cfg.ID],
			})
		}

		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "ok", Data: items})
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
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "started"})
	case "stop":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		if err := s.manager.Stop(taskID); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "stopped"})
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
		tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
		if tail <= 0 {
			tail = 200
		}

		data, err := readLogTail(filepath.Join("data", "logs", taskID, logType+".log"), tail)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "ok", Data: map[string]string{"content": data}})
	case "clear-logs":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, response{Code: 1, Msg: "method not allowed"})
			return
		}
		if err := s.manager.ClearLogs(taskID); err != nil {
			writeJSON(w, http.StatusBadRequest, response{Code: 1, Msg: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, response{Code: 0, Msg: "logs cleared"})
	default:
		http.NotFound(w, r)
	}
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

func writeJSON(w http.ResponseWriter, status int, payload response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

const indexHTML = `
<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Tray Command Manager</title>
  <style>
    :root {
      --bg: #f8f9fa;
      --surface: #ffffff;
      --border: #e9ecef;
      --text-main: #212529;
      --text-muted: #6c757d;
      --primary: #0d6efd;
      --primary-hover: #0b5ed7;
      --danger: #dc3545;
      --danger-hover: #c82333;
      --success: #198754;
      --warning: #ffc107;
      --radius: 12px;
      --shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
      --modal-shadow: 0 10px 25px rgba(0, 0, 0, 0.2);
      --font: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
    }
    
    * { box-sizing: border-box; margin: 0; padding: 0; }
    
    body {
      font-family: var(--font);
      background-color: var(--bg);
      color: var(--text-main);
      line-height: 1.5;
      -webkit-font-smoothing: antialiased;
      display: flex;
      flex-direction: column;
      min-height: 100vh;
    }
    
    .container {
      max-width: 1200px;
      margin: 0 auto;
      padding: 2rem 1.5rem 0;
      flex-grow: 1;
      width: 100%;
    }
    
    .header {
      display: flex;
      justify-content: space-between;
      align-items: flex-end;
      margin-bottom: 2rem;
      border-bottom: 1px solid #dee2e6;
    }
    
    .header-titles {
      padding-bottom: 1.25rem;
    }
    
    .header-titles h1 {
      font-size: 1.75rem;
      font-weight: 700;
      color: var(--text-main);
      margin-bottom: 0.25rem;
    }
    
    .header-titles p {
      color: var(--text-muted);
      font-size: 0.95rem;
    }

    /* Tabs */
    .nav-tabs {
      display: flex;
      gap: 0.5rem;
      margin-bottom: -1px;
    }

    .nav-link {
      background: none;
      border: none;
      border-bottom: 3px solid transparent;
      padding: 0.75rem 1.25rem;
      font-size: 1.05rem;
      font-weight: 600;
      color: var(--text-muted);
      cursor: pointer;
      transition: all 0.2s;
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }

    .nav-link:hover {
      color: var(--text-main);
      border-bottom-color: #dee2e6;
    }

    .nav-link.active {
      color: var(--primary);
      border-bottom-color: var(--primary);
    }

    .tab-content {
      display: none;
    }
    
    .tab-content.active {
      display: block;
      animation: fadeIn 0.3s ease-in-out;
    }

    @keyframes fadeIn {
      from { opacity: 0; transform: translateY(5px); }
      to { opacity: 1; transform: translateY(0); }
    }
    
    .card {
      background: var(--surface);
      border-radius: var(--radius);
      box-shadow: var(--shadow);
      border: 1px solid var(--border);
      overflow: hidden;
    }

    .card-header {
      padding: 1rem 1.5rem;
      border-bottom: 1px solid var(--border);
      display: flex;
      justify-content: space-between;
      align-items: center;
      background: rgba(0,0,0,0.015);
    }
    
    .card-title {
      font-weight: 600;
      font-size: 1.05rem;
      margin: 0;
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }
    
    .card-body {
      padding: 1.5rem;
    }
    
    .card-body-no-pad {
      padding: 0;
    }
    
    /* Task List */
    .task-item {
      display: grid;
      grid-template-columns: minmax(0, 2fr) minmax(0, 1fr) minmax(0, 1.2fr) auto;
      gap: 1rem;
      align-items: center;
      padding: 1.25rem 1.5rem;
      border-bottom: 1px solid var(--border);
      transition: background-color 0.2s;
    }
    
    .task-item:last-child {
      border-bottom: none;
    }
    
    .task-item:hover {
      background-color: #f8f9fa;
    }
    
    .task-name {
      font-weight: 600;
      font-size: 1.05rem;
      margin-bottom: 0.25rem;
      color: var(--primary);
    }
    
    .task-desc, .task-meta {
      color: var(--text-muted);
      font-size: 0.85rem;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    
    /* Badges */
    .badge {
      display: inline-flex;
      align-items: center;
      padding: 0.35em 0.65em;
      font-size: 0.75em;
      font-weight: 600;
      border-radius: 0.25rem;
      text-transform: uppercase;
      letter-spacing: 0.05em;
    }
    
    .status-running { background: #d1e7dd; color: #0f5132; }
    .status-starting, .status-stopping { background: #fff3cd; color: #664d03; }
    .status-stopped, .status-exited { background: #e2e3e5; color: #41464b; }
    .status-failed { background: #f8d7da; color: #842029; }
    
    /* Buttons */
    .btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      padding: 0.5rem 1rem;
      font-size: 0.875rem;
      font-weight: 500;
      border-radius: 6px;
      border: 1px solid var(--border);
      cursor: pointer;
      transition: all 0.2s;
      background: #fff;
      color: var(--text-main);
      text-decoration: none;
    }
    
    .btn:hover:not(:disabled) {
      background: #e9ecef;
      border-color: #dee2e6;
    }
    
    .btn:disabled {
      opacity: 0.65;
      cursor: not-allowed;
    }
    
    .btn-primary {
      background: var(--primary);
      border-color: var(--primary);
      color: white;
    }
    
    .btn-primary:hover:not(:disabled) {
      background: var(--primary-hover);
      border-color: var(--primary-hover);
      color: white;
    }
    
    .btn-danger {
      background: #fff;
      border-color: var(--danger);
      color: var(--danger);
    }
    
    .btn-danger:hover:not(:disabled) {
      background: var(--danger);
      color: white;
    }
    
    .btn-sm {
      padding: 0.3rem 0.6rem;
      font-size: 0.8rem;
      border-radius: 4px;
    }
    
    .actions-group {
      display: flex;
      gap: 0.5rem;
      flex-wrap: wrap;
      justify-content: flex-end;
    }
    
    /* Forms */
    .form-group {
      margin-bottom: 1.25rem;
    }
    
    .form-label {
      display: block;
      margin-bottom: 0.4rem;
      font-size: 0.875rem;
      font-weight: 600;
      color: var(--text-main);
    }
    
    .form-control {
      display: block;
      width: 100%;
      padding: 0.5rem 0.75rem;
      font-size: 0.95rem;
      font-family: inherit;
      line-height: 1.5;
      color: var(--text-main);
      background-color: #fff;
      border: 1px solid #ced4da;
      border-radius: 6px;
      transition: border-color 0.15s, box-shadow 0.15s;
    }
    
    .form-control:focus {
      outline: 0;
      border-color: #86b7fe;
      box-shadow: 0 0 0 0.25rem rgba(13, 110, 253, 0.25);
    }
    
    .form-control:disabled {
      background-color: #e9ecef;
      opacity: 1;
    }
    
    .form-select {
      appearance: none;
      background-image: url("data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'%3e%3cpath fill='none' stroke='%23343a40' stroke-linecap='round' stroke-linejoin='round' stroke-width='2' d='M2 5l6 6 6-6'/%3e%3c/svg%3e");
      background-repeat: no-repeat;
      background-position: right 0.75rem center;
      background-size: 16px 12px;
      padding-right: 2.25rem;
    }
    
    .form-check {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      margin-bottom: 0.75rem;
    }
    
    .form-check-input {
      width: 1rem;
      height: 1rem;
      margin: 0;
      background-color: #fff;
      border: 1px solid #adb5bd;
      border-radius: 0.25em;
      cursor: pointer;
    }
    
    .form-check-input:checked {
      background-color: var(--primary);
      border-color: var(--primary);
    }
    
    .form-check-label {
      font-size: 0.9rem;
      cursor: pointer;
      user-select: none;
    }
    
    /* Logs */
    .logs-toolbar {
      display: flex;
      flex-wrap: wrap;
      gap: 1rem;
      align-items: center;
      margin-bottom: 1rem;
      background: #f8f9fa;
      padding: 0.75rem 1rem;
      border-radius: 6px;
      border: 1px solid var(--border);
    }
    
    .toolbar-item {
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }
    
    .log-viewer {
      background: #1e1e1e;
      color: #d4d4d4;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
      font-size: 0.85rem;
      padding: 1rem;
      border-radius: 6px;
      height: 550px;
      overflow-y: auto;
      white-space: pre-wrap;
      word-break: break-all;
      margin: 0;
      box-shadow: inset 0 2px 4px rgba(0,0,0,0.2);
    }
    
    .error-text {
      color: var(--danger);
      font-size: 0.8rem;
      margin-top: 0.35rem;
      word-break: break-word;
      display: block;
    }
    
    .empty-state {
      padding: 4rem;
      text-align: center;
      color: var(--text-muted);
    }

    /* Modal */
    .modal-backdrop {
      position: fixed;
      top: 0; left: 0; width: 100vw; height: 100vh;
      background: rgba(0, 0, 0, 0.5);
      backdrop-filter: blur(2px);
      z-index: 1040;
      display: none;
      align-items: center;
      justify-content: center;
      opacity: 0;
      transition: opacity 0.2s ease-in-out;
    }
    .modal-backdrop.show {
      display: flex;
      opacity: 1;
    }

    .modal-dialog {
      background: var(--surface);
      border-radius: var(--radius);
      box-shadow: var(--modal-shadow);
      width: 100%;
      max-width: 550px;
      max-height: 90vh;
      display: flex;
      flex-direction: column;
      transform: translateY(-20px);
      transition: transform 0.2s ease-in-out;
    }
    .modal-backdrop.show .modal-dialog {
      transform: translateY(0);
    }

    .modal-header {
      padding: 1.25rem 1.5rem;
      border-bottom: 1px solid var(--border);
      display: flex;
      justify-content: space-between;
      align-items: center;
    }

    .modal-title {
      font-size: 1.25rem;
      font-weight: 600;
      margin: 0;
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }

    .modal-close {
      background: none;
      border: none;
      font-size: 1.5rem;
      line-height: 1;
      color: var(--text-muted);
      cursor: pointer;
      padding: 0;
    }
    .modal-close:hover {
      color: var(--text-main);
    }

    .modal-body {
      padding: 1.5rem;
      overflow-y: auto;
    }

    .modal-footer {
      padding: 1.25rem 1.5rem;
      border-top: 1px solid var(--border);
      display: flex;
      justify-content: flex-end;
      gap: 0.75rem;
      background: rgba(0,0,0,0.015);
      border-bottom-left-radius: var(--radius);
      border-bottom-right-radius: var(--radius);
    }

    /* Footer */
    .site-footer {
      margin-top: 3rem;
      padding: 1.5rem 0;
      border-top: 1px solid var(--border);
      background-color: rgba(255, 255, 255, 0.5);
    }

    .footer-inner {
      max-width: 1200px;
      margin: 0 auto;
      padding: 0 1.5rem;
      display: grid;
      grid-template-columns: 1fr auto 1fr;
      align-items: center;
      color: var(--text-muted);
      font-size: 0.85rem;
    }
    
    .footer-left {
      /* Empty for balance */
    }
    
    .footer-center {
      text-align: center;
    }
    
    .footer-right {
      text-align: right;
    }

    .version-badge {
      display: inline-block;
      background: var(--border);
      padding: 0.2rem 0.6rem;
      border-radius: 999px;
      font-size: 0.75rem;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      color: var(--text-main);
    }
    
    @media (max-width: 992px) {
      .header {
        flex-direction: column;
        align-items: flex-start;
        gap: 1rem;
      }
      .nav-tabs {
        width: 100%;
      }
      .task-item {
        grid-template-columns: 1fr;
        gap: 0.75rem;
      }
      .actions-group {
        justify-content: flex-start;
        margin-top: 0.5rem;
      }
      .footer-inner {
        grid-template-columns: 1fr;
        gap: 0.75rem;
      }
      .footer-left {
        display: none;
      }
      .footer-right, .footer-center {
        text-align: center;
      }
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <div class="header-titles">
        <h1>Tray Command Manager</h1>
        <p style="margin:0;">托盘负责快速控制，浏览器负责命令管理与日志查看。</p>
      </div>
      
      <!-- Tabs Navigation -->
      <div class="nav-tabs">
        <button class="nav-link active" onclick="switchTab('tasks')" id="tab-btn-tasks">
          <svg style="width:18px;height:18px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="7" width="20" height="14" rx="2" ry="2"/><path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16"/></svg>
          任务列表
        </button>
        <button class="nav-link" onclick="switchTab('logs')" id="tab-btn-logs">
          <svg style="width:18px;height:18px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
          日志查看
        </button>
      </div>
    </div>

    <!-- Tab 1: Task List -->
    <div class="tab-content active" id="tab-tasks">
      <div class="card">
        <div class="card-header">
          <div class="card-title">
            所有任务
          </div>
          <div style="display: flex; gap: 0.75rem;">
            <button class="btn btn-sm btn-primary" onclick="openNewTaskModal()">
              <svg style="width:14px;height:14px;margin-right:4px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
              添加任务
            </button>
            <button class="btn btn-sm" onclick="loadTasks()">
              <svg style="width:14px;height:14px;margin-right:4px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 2v6h-6M3 12a9 9 0 0 1 15-6.7L21 8M3 22v-6h6M21 12a9 9 0 0 1-15 6.7L3 16"/></svg>
              刷新
            </button>
          </div>
        </div>
        <div class="card-body card-body-no-pad" id="task-list">
          <div class="empty-state">加载中...</div>
        </div>
      </div>
    </div>

    <!-- Tab 2: Logs -->
    <div class="tab-content" id="tab-logs">
      <div class="card">
        <div class="card-body">
          <div class="logs-toolbar">
            <div class="toolbar-item">
              <select id="log-task" class="form-control form-select" style="width: auto; min-width: 200px;"></select>
            </div>
            <div class="toolbar-item">
              <select id="log-type" class="form-control form-select" style="width: auto;">
                <option value="stdout">stdout</option>
                <option value="stderr">stderr</option>
              </select>
            </div>
            <div class="toolbar-item">
              <label class="form-check-label" style="display:flex;align-items:center;gap:4px;">
                <input id="log-auto-refresh" type="checkbox" class="form-check-input" checked>
                自动刷新
              </label>
            </div>
            <div class="toolbar-item">
              <label class="form-check-label" style="display:flex;align-items:center;gap:4px;">
                <input id="log-follow" type="checkbox" class="form-check-input" checked>
                跟随到底部
              </label>
            </div>
            <div style="flex-grow: 1;"></div>
            <button class="btn btn-sm" onclick="loadLogs()">读取</button>
            <button class="btn btn-sm btn-danger" onclick="clearLogs()">清空</button>
          </div>
          <pre id="logs" class="log-viewer"></pre>
        </div>
      </div>
    </div>
  </div>

  <footer class="site-footer">
    <div class="footer-inner">
      <div class="footer-left"></div>
      <div class="footer-center">
        &copy; <span id="current-year"></span> Tray Command Manager. All rights reserved.
      </div>
      <div class="footer-right">
        <span class="version-badge">v1.0.0</span>
      </div>
    </div>
  </footer>

  <!-- Task Edit Modal -->
  <div id="task-modal" class="modal-backdrop" onclick="handleBackdropClick(event)">
    <div class="modal-dialog">
      <div class="modal-header">
        <h3 class="modal-title" id="modal-title">新增任务</h3>
        <button class="modal-close" onclick="closeModal()">&times;</button>
      </div>
      <div class="modal-body">
        <div class="form-group">
          <label class="form-label" for="task-id">任务 ID</label>
          <input id="task-id" class="form-control" placeholder="例如: openlist">
        </div>
        <div class="form-group">
          <label class="form-label" for="task-name">显示名称</label>
          <input id="task-name" class="form-control" placeholder="例如: OpenList 服务">
        </div>
        <div class="form-group">
          <label class="form-label" for="task-program">可执行程序</label>
          <input id="task-program" class="form-control" placeholder="例如: D:\SoftWare\openlist.exe">
        </div>
        <div class="form-group">
          <label class="form-label" for="task-args">运行参数</label>
          <input id="task-args" class="form-control" placeholder="例如: server --port 5244">
        </div>
        <div class="form-group">
          <label class="form-label" for="task-workdir">工作目录</label>
          <input id="task-workdir" class="form-control" placeholder="留空则使用当前目录">
        </div>
        <div class="form-group">
          <label class="form-label" for="task-timeout">停止超时 (秒)</label>
          <input id="task-timeout" class="form-control" type="number" min="1" value="8">
        </div>
        
        <div class="form-check">
          <input id="task-autostart" type="checkbox" class="form-check-input">
          <label class="form-check-label" for="task-autostart">跟随托盘自动启动</label>
        </div>
        <div class="form-check">
          <input id="task-restart" type="checkbox" class="form-check-input">
          <label class="form-check-label" for="task-restart">异常退出后自动重启</label>
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn" onclick="closeModal()">取消</button>
        <button class="btn btn-primary" id="btn-save" onclick="saveTask()">保存任务</button>
      </div>
    </div>
  </div>

  <script>
    let currentTasks = [];
    let editingTaskId = '';

    document.getElementById('current-year').textContent = new Date().getFullYear();

    async function api(url, options = {}) {
      try {
        const res = await fetch(url, options);
        return await res.json();
      } catch (err) {
        console.error(err);
        return { code: -1, msg: err.message };
      }
    }

    function escapeHTML(value) {
      return String(value || '')
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
        .replaceAll("'", '&#39;');
    }

    function isRunningStatus(status) {
      return status === 'running' || status === 'starting' || status === 'stopping';
    }

    function splitArgs(input) {
      return String(input || '')
        .trim()
        .split(/\s+/)
        .filter(Boolean);
    }

    // Tabs Control
    function switchTab(tabId) {
      document.querySelectorAll('.tab-content').forEach(el => el.classList.remove('active'));
      document.querySelectorAll('.nav-link').forEach(el => el.classList.remove('active'));
      
      document.getElementById('tab-' + tabId).classList.add('active');
      document.getElementById('tab-btn-' + tabId).classList.add('active');

      if (tabId === 'logs') {
        loadLogs();
      }
    }

    // Modal Control
    function openModal() {
      const modal = document.getElementById('task-modal');
      modal.style.display = 'flex';
      // Trigger reflow for transition
      modal.offsetHeight;
      modal.classList.add('show');
    }

    function closeModal() {
      const modal = document.getElementById('task-modal');
      modal.classList.remove('show');
      setTimeout(() => {
        modal.style.display = 'none';
      }, 200);
    }

    function handleBackdropClick(e) {
      if (e.target.id === 'task-modal') {
        closeModal();
      }
    }

    function openNewTaskModal() {
      editingTaskId = '';
      document.getElementById('modal-title').innerHTML = '<svg style="width:18px;height:18px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 20h9M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/></svg> 新增任务';
      document.getElementById('task-id').disabled = false;
      document.getElementById('task-id').value = '';
      document.getElementById('task-name').value = '';
      document.getElementById('task-program').value = '';
      document.getElementById('task-args').value = '';
      document.getElementById('task-workdir').value = '';
      document.getElementById('task-timeout').value = 8;
      document.getElementById('task-autostart').checked = false;
      document.getElementById('task-restart').checked = false;
      openModal();
    }

    function openEditTaskModal(task) {
      editingTaskId = task.id;
      document.getElementById('modal-title').innerHTML = '<svg style="width:18px;height:18px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg> 编辑任务: ' + escapeHTML(task.name);
      document.getElementById('task-id').value = task.id || '';
      document.getElementById('task-id').disabled = true;
      document.getElementById('task-name').value = task.name || '';
      document.getElementById('task-program').value = task.program || '';
      document.getElementById('task-args').value = (task.args || []).join(' ');
      document.getElementById('task-workdir').value = task.workdir || '';
      document.getElementById('task-timeout').value = task.stop_timeout_sec || 8;
      document.getElementById('task-autostart').checked = !!task.autostart;
      document.getElementById('task-restart').checked = !!task.restart_on_crash;
      openModal();
    }

    function renderTasks(items) {
      const root = document.getElementById('task-list');
      if (items.length === 0) {
        root.innerHTML = '<div class="empty-state">暂无任务，请点击右上方“添加任务”按钮。</div>';
      } else {
        root.innerHTML = '';
      }
      
      currentTasks = items;

      const taskSelect = document.getElementById('log-task');
      const currentLogTask = taskSelect.value;
      taskSelect.innerHTML = '';

      items.forEach(item => {
        const status = item.status.status || 'stopped';
        const isRunning = isRunningStatus(status);
        const toggleAction = isRunning ? 'stop' : 'start';
        
        let toggleIcon = isRunning 
          ? '<svg style="width:14px;height:14px;margin-right:4px;" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="6" width="12" height="12"/></svg>'
          : '<svg style="width:14px;height:14px;margin-right:4px;" viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"/></svg>';
        const toggleLabel = isRunning ? '停止' : '启动';
        const toggleClass = isRunning ? 'btn-danger' : 'btn-primary';
        
        const lastError = item.status.last_error ? '<span class="error-text" title="' + escapeHTML(item.status.last_error) + '">' + escapeHTML(item.status.last_error) + '</span>' : '';
        const disabledActions = isRunning;

        const option = document.createElement('option');
        option.value = item.task.id;
        option.textContent = item.task.name;
        taskSelect.appendChild(option);

        const row = document.createElement('div');
        row.className = 'task-item';
        row.innerHTML =
          '<div>' +
            '<div class="task-name">' + escapeHTML(item.task.name) + ' <span style="color:var(--text-muted);font-weight:normal;font-size:0.85rem;">(' + escapeHTML(item.task.id) + ')</span></div>' +
            '<div class="task-desc" title="' + escapeHTML(item.task.program) + ' ' + escapeHTML((item.task.args || []).join(' ')) + '">' + 
               '<svg style="width:14px;height:14px;vertical-align:middle;margin-right:4px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>' +
               escapeHTML(item.task.program) + ' ' + escapeHTML((item.task.args || []).join(' ')) + 
            '</div>' +
            '<div class="task-meta" title="' + escapeHTML(item.task.workdir || '.') + '">' + 
               '<svg style="width:14px;height:14px;vertical-align:middle;margin-right:4px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>' +
               escapeHTML(item.task.workdir || '当前目录') + 
            '</div>' +
          '</div>' +
          '<div>' +
            '<div class="badge status-' + status + '">' + status + '</div>' +
            (item.status.pid ? '<div class="task-meta" style="margin-top:4px;">PID: ' + item.status.pid + '</div>' : '') +
            lastError +
          '</div>' +
          '<div>' +
            '<div class="task-meta">自动启动: <strong>' + (item.task.autostart ? '是' : '否') + '</strong></div>' +
            '<div class="task-meta" style="margin-top:4px;">异常重启: <strong>' + (item.task.restart_on_crash ? '是' : '否') + '</strong></div>' +
          '</div>' +
          '<div class="actions-group">' +
            '<button class="btn btn-sm ' + toggleClass + '" data-id="' + item.task.id + '" data-action="' + toggleAction + '">' + toggleIcon + toggleLabel + '</button>' +
            '<button class="btn btn-sm" data-id="' + item.task.id + '" data-action="edit"' + (disabledActions ? ' disabled title="运行中不可编辑"' : '') + '>编辑</button>' +
            '<button class="btn btn-sm btn-danger" data-id="' + item.task.id + '" data-action="delete"' + (disabledActions ? ' disabled title="运行中不可删除"' : '') + '>删除</button>' +
            '<button class="btn btn-sm" data-id="' + item.task.id + '" data-action="logs">看日志</button>' +
          '</div>';
        root.appendChild(row);
      });

      if (items.length > 0) {
        if (currentLogTask && items.some(x => x.task.id === currentLogTask)) {
          taskSelect.value = currentLogTask;
        } else {
          taskSelect.value = items[0].task.id;
        }
      }

      root.querySelectorAll('button[data-action]').forEach(btn => {
        btn.addEventListener('click', async () => {
          if (btn.disabled) return;
          
          const id = btn.dataset.id;
          const action = btn.dataset.action;

          if (action === 'logs') {
            document.getElementById('log-task').value = id;
            switchTab('logs');
            return;
          }

          if (action === 'edit') {
            const item = currentTasks.find(x => x.task.id === id);
            if (item) openEditTaskModal(item.task);
            return;
          }

          if (action === 'delete') {
            if (!confirm('确认删除任务 "' + id + '" 吗？')) return;
            const result = await api('/api/tasks/' + id, { method: 'DELETE' });
            if (result.code !== 0) {
              alert(result.msg || '删除失败');
              return;
            }
            await loadTasks();
            await loadLogs();
            if (editingTaskId === id) closeModal();
            return;
          }

          // start or stop
          const originalText = btn.innerHTML;
          btn.innerHTML = '处理中...';
          btn.disabled = true;
          
          const result = await api('/api/tasks/' + id + '/' + action, { method: 'POST' });
          if (result.code !== 0) {
            alert(result.msg || '操作失败');
          }
          await loadTasks();
          await loadLogs();
        });
      });
    }

    async function saveTask() {
      const id = document.getElementById('task-id').value.trim();
      if (!id) {
        alert("请输入任务 ID");
        return;
      }
      
      const payload = {
        id: id,
        name: document.getElementById('task-name').value.trim() || id,
        program: document.getElementById('task-program').value.trim(),
        args: splitArgs(document.getElementById('task-args').value),
        workdir: document.getElementById('task-workdir').value.trim(),
        env: [],
        autostart: document.getElementById('task-autostart').checked,
        restart_on_crash: document.getElementById('task-restart').checked,
        stop_timeout_sec: parseInt(document.getElementById('task-timeout').value || '8', 10)
      };

      if (!payload.program) {
        alert("请输入可执行程序");
        return;
      }

      const isEdit = !!editingTaskId;
      const url = isEdit ? '/api/tasks/' + editingTaskId : '/api/tasks';
      const method = isEdit ? 'PUT' : 'POST';

      const btn = document.getElementById('btn-save');
      const originalText = btn.innerHTML;
      btn.innerHTML = '保存中...';
      btn.disabled = true;

      const result = await api(url, {
        method: method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });

      btn.innerHTML = originalText;
      btn.disabled = false;

      if (result.code !== 0) {
        alert(result.msg || '保存失败');
        return;
      }

      closeModal();
      await loadTasks();
    }

    async function loadTasks() {
      const data = await api('/api/tasks');
      if (data && data.code === 0) {
        renderTasks(data.data || []);
      }
    }

    async function loadLogs() {
      // 只有当前在日志 Tab 时才去查日志接口
      if (!document.getElementById('tab-logs').classList.contains('active')) {
        return;
      }
      
      const taskId = document.getElementById('log-task').value;
      const type = document.getElementById('log-type').value;
      const follow = document.getElementById('log-follow').checked;
      const logsEl = document.getElementById('logs');
      if (!taskId) {
        logsEl.textContent = '请选择一个任务查看日志...';
        return;
      }
      
      const nearBottom = logsEl.scrollTop + logsEl.clientHeight >= logsEl.scrollHeight - 40;
      const data = await api('/api/tasks/' + taskId + '/logs?type=' + type + '&tail=400');
      
      if (data && data.code === 0) {
        logsEl.textContent = (data.data && data.data.content) || '';
      } else {
        logsEl.textContent = '无法读取日志: ' + (data.msg || '未知错误');
      }
      
      if (follow || nearBottom) {
        logsEl.scrollTop = logsEl.scrollHeight;
      }
    }

    async function clearLogs() {
      const taskId = document.getElementById('log-task').value;
      if (!taskId) return;
      if (!confirm('确认清空该任务的日志吗？')) return;
      
      const result = await api('/api/tasks/' + taskId + '/clear-logs', { method: 'POST' });
      if (result.code !== 0) {
        alert(result.msg || '清空日志失败');
        return;
      }
      document.getElementById('logs').textContent = '';
      await loadLogs();
    }

    // Init
    loadTasks();
    
    // Auto refresh
    setInterval(loadTasks, 5000);
    setInterval(() => {
      if (document.getElementById('log-auto-refresh').checked) {
        loadLogs();
      }
    }, 2000);
    
    // Trigger task selection change
    document.getElementById('log-task').addEventListener('change', loadLogs);
    document.getElementById('log-type').addEventListener('change', loadLogs);
  </script>
</body>
</html>`
