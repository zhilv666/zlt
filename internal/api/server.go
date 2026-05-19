package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Tray Command Manager</title>
  <style>
    :root {
      --bg: #f2efe8;
      --panel: #fffdf8;
      --line: #d8cfbe;
      --ink: #27211b;
      --sub: #766a5d;
      --accent: #b85c38;
      --accent-2: #2f6c60;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", "Microsoft YaHei", sans-serif;
      color: var(--ink);
      background:
        radial-gradient(circle at top left, #f8d9b8 0, transparent 30%),
        radial-gradient(circle at bottom right, #d7ebe4 0, transparent 28%),
        var(--bg);
    }
    .wrap {
      max-width: 1180px;
      margin: 0 auto;
      padding: 32px 20px 48px;
    }
    .hero {
      display: flex;
      justify-content: space-between;
      gap: 24px;
      align-items: end;
      margin-bottom: 24px;
    }
    h1 {
      margin: 0;
      font-size: 34px;
      line-height: 1.1;
    }
    .sub {
      color: var(--sub);
      margin-top: 8px;
    }
    .layout {
      display: grid;
      grid-template-columns: 1.5fr 0.9fr;
      gap: 20px;
    }
    .panel {
      background: rgba(255,255,255,0.8);
      backdrop-filter: blur(10px);
      border: 1px solid var(--line);
      border-radius: 18px;
      padding: 18px;
      box-shadow: 0 18px 40px rgba(39,33,27,0.08);
    }
    .task {
      display: grid;
      grid-template-columns: 1.4fr 1fr 1fr auto;
      gap: 14px;
      align-items: center;
      padding: 14px 0;
      border-top: 1px solid var(--line);
    }
    .task:first-of-type {
      border-top: 0;
    }
    .name {
      font-weight: 700;
      font-size: 18px;
    }
    .meta {
      color: var(--sub);
      font-size: 13px;
      margin-top: 4px;
    }
    .status {
      font-weight: 700;
    }
    .status.running { color: var(--accent-2); }
    .status.starting, .status.stopping { color: var(--accent); }
    .status.stopped, .status.exited { color: var(--sub); }
    .status.failed { color: #b42318; }
    .error {
      margin-top: 6px;
      color: #b42318;
      font-size: 12px;
      line-height: 1.5;
      word-break: break-word;
    }
    .actions {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      justify-content: flex-end;
    }
    input, textarea, button, select {
      border: 1px solid var(--line);
      background: var(--panel);
      color: var(--ink);
      border-radius: 14px;
      padding: 10px 12px;
      width: 100%;
    }
    button, select {
      border-radius: 999px;
      width: auto;
      cursor: pointer;
    }
    button.primary {
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
    }
    button.warn {
      border-color: #d46a4d;
      color: #b13d1b;
    }
    .field {
      margin-bottom: 12px;
    }
    .field label {
      display: block;
      margin-bottom: 6px;
      font-size: 13px;
      color: var(--sub);
    }
    .inline {
      display: flex;
      gap: 10px;
      align-items: center;
      flex-wrap: wrap;
    }
    .checkbox {
      display: flex;
      gap: 8px;
      align-items: center;
      margin: 8px 0;
      color: var(--ink);
    }
    .checkbox input {
      width: auto;
      margin: 0;
    }
    pre {
      background: #201b16;
      color: #f8eadb;
      padding: 16px;
      border-radius: 14px;
      height: 420px;
      max-height: 420px;
      overflow-x: auto;
      overflow-y: auto;
      white-space: pre-wrap;
      word-break: break-word;
      margin: 0;
    }
    .toolbar {
      display: flex;
      gap: 10px;
      margin-bottom: 12px;
      align-items: center;
      justify-content: space-between;
      flex-wrap: wrap;
    }
    .panel-title {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 10px;
    }
    .hint {
      color: var(--sub);
      font-size: 12px;
    }
    @media (max-width: 980px) {
      .layout {
        grid-template-columns: 1fr;
      }
    }
    @media (max-width: 820px) {
      .hero, .toolbar, .task {
        display: block;
      }
      .task > div {
        margin-bottom: 10px;
      }
      .actions {
        justify-content: flex-start;
      }
    }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="hero">
      <div>
        <h1>Tray Command Manager</h1>
        <div class="sub">托盘负责快速控制，浏览器负责命令管理与日志查看。</div>
      </div>
      <button onclick="loadTasks()">刷新</button>
    </div>

    <div class="layout">
      <div>
        <div class="panel">
          <div class="panel-title">
            <strong>任务列表</strong>
            <span class="hint">运行中的任务不能编辑或删除，先停止再操作。</span>
          </div>
          <div id="task-list"></div>
        </div>

        <div class="panel" style="margin-top: 20px;">
        <div class="toolbar">
          <strong>日志查看</strong>
          <div>
            <select id="log-task"></select>
            <select id="log-type">
              <option value="stdout">stdout</option>
              <option value="stderr">stderr</option>
            </select>
            <label class="checkbox" style="display:inline-flex; margin:0 8px 0 0;">
              <input id="log-auto-refresh" type="checkbox" checked>
              <span>自动刷新</span>
            </label>
            <label class="checkbox" style="display:inline-flex; margin:0 8px 0 0;">
              <input id="log-follow" type="checkbox" checked>
              <span>跟随到底部</span>
            </label>
            <button onclick="loadLogs()">读取日志</button>
            <button class="warn" onclick="clearLogs()">清空日志</button>
          </div>
        </div>
        <pre id="logs"></pre>
        </div>
      </div>

      <div class="panel">
        <div class="panel-title">
          <strong id="form-title">新增任务</strong>
          <button onclick="resetForm()">清空</button>
        </div>
        <div class="field">
          <label for="task-id">ID</label>
          <input id="task-id" placeholder="例如: openlist">
        </div>
        <div class="field">
          <label for="task-name">名称</label>
          <input id="task-name" placeholder="例如: OpenList">
        </div>
        <div class="field">
          <label for="task-program">程序</label>
          <input id="task-program" placeholder="例如: openlist.exe 或 D:\SoftWare\OpenList\openlist.exe">
        </div>
        <div class="field">
          <label for="task-args">参数</label>
          <input id="task-args" placeholder="例如: server --port 5244">
        </div>
        <div class="field">
          <label for="task-workdir">工作目录</label>
          <input id="task-workdir" placeholder="例如: D:\SoftWare\OpenList">
        </div>
        <div class="field">
          <label for="task-timeout">停止超时秒数</label>
          <input id="task-timeout" type="number" min="1" value="8">
        </div>
        <label class="checkbox">
          <input id="task-autostart" type="checkbox">
          <span>自动启动</span>
        </label>
        <label class="checkbox">
          <input id="task-restart" type="checkbox">
          <span>异常退出自动重启</span>
        </label>
        <div class="inline" style="margin-top: 16px;">
          <button class="primary" onclick="saveTask()">保存任务</button>
        </div>
      </div>
    </div>
  </div>

  <script>
    let currentTasks = [];
    let editingTaskId = '';

    async function api(url, options = {}) {
      const res = await fetch(url, options);
      return res.json();
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

    function renderTasks(items) {
      const root = document.getElementById('task-list');
      root.innerHTML = '';
      currentTasks = items;

      const taskSelect = document.getElementById('log-task');
      const currentLogTask = taskSelect.value;
      taskSelect.innerHTML = '';

      items.forEach(item => {
        const status = item.status.status || 'stopped';
        const toggleAction = isRunningStatus(status) ? 'stop' : 'start';
        const toggleLabel = isRunningStatus(status) ? '停止' : '启动';
        const toggleClass = isRunningStatus(status) ? '' : 'primary';
        const lastError = item.status.last_error ? '<div class="error">' + escapeHTML(item.status.last_error) + '</div>' : '';
        const disabledActions = isRunningStatus(status);

        const option = document.createElement('option');
        option.value = item.task.id;
        option.textContent = item.task.name;
        taskSelect.appendChild(option);

        const row = document.createElement('div');
        row.className = 'task';
        row.innerHTML =
          '<div>' +
            '<div class="name">' + escapeHTML(item.task.name) + '</div>' +
            '<div class="meta">' + escapeHTML(item.task.program) + ' ' + escapeHTML((item.task.args || []).join(' ')) + '</div>' +
            '<div class="meta">workdir: ' + escapeHTML(item.task.workdir || '.') + '</div>' +
          '</div>' +
          '<div>' +
            '<div class="status ' + status + '">' + status + '</div>' +
            '<div class="meta">PID: ' + (item.status.pid || '-') + '</div>' +
            lastError +
          '</div>' +
          '<div>' +
            '<div class="meta">AutoStart: ' + (item.task.autostart ? 'Yes' : 'No') + '</div>' +
            '<div class="meta">RestartOnCrash: ' + (item.task.restart_on_crash ? 'Yes' : 'No') + '</div>' +
          '</div>' +
          '<div class="actions">' +
            '<button class="' + toggleClass + '" data-id="' + item.task.id + '" data-action="' + toggleAction + '">' + toggleLabel + '</button>' +
            '<button data-id="' + item.task.id + '" data-action="edit"' + (disabledActions ? ' disabled' : '') + '>编辑</button>' +
            '<button class="warn" data-id="' + item.task.id + '" data-action="delete"' + (disabledActions ? ' disabled' : '') + '>删除</button>' +
            '<button data-id="' + item.task.id + '" data-action="logs">日志</button>' +
          '</div>';
        root.appendChild(row);
      });

      if (items.length > 0) {
        taskSelect.value = currentLogTask || items[0].task.id;
      }

      root.querySelectorAll('button[data-action]').forEach(btn => {
        btn.addEventListener('click', async () => {
          const id = btn.dataset.id;
          const action = btn.dataset.action;

          if (action === 'logs') {
            document.getElementById('log-task').value = id;
            await loadLogs();
            return;
          }

          if (action === 'edit') {
            const item = currentTasks.find(x => x.task.id === id);
            if (item) {
              fillForm(item.task);
            }
            return;
          }

          if (action === 'delete') {
            if (!confirm('确认删除任务 ' + id + ' ?')) {
              return;
            }
            const result = await api('/api/tasks/' + id, { method: 'DELETE' });
            if (result.code !== 0) {
              alert(result.msg || '删除失败');
            }
            await loadTasks();
            await loadLogs();
            if (editingTaskId === id) {
              resetForm();
            }
            return;
          }

          const result = await api('/api/tasks/' + id + '/' + action, { method: 'POST' });
          if (result.code !== 0) {
            alert(result.msg || '操作失败');
          }
          await loadTasks();
          await loadLogs();
        });
      });
    }

    function fillForm(task) {
      editingTaskId = task.id;
      document.getElementById('form-title').textContent = '编辑任务';
      document.getElementById('task-id').value = task.id || '';
      document.getElementById('task-id').disabled = true;
      document.getElementById('task-name').value = task.name || '';
      document.getElementById('task-program').value = task.program || '';
      document.getElementById('task-args').value = (task.args || []).join(' ');
      document.getElementById('task-workdir').value = task.workdir || '.';
      document.getElementById('task-timeout').value = task.stop_timeout_sec || 8;
      document.getElementById('task-autostart').checked = !!task.autostart;
      document.getElementById('task-restart').checked = !!task.restart_on_crash;
    }

    function resetForm() {
      editingTaskId = '';
      document.getElementById('form-title').textContent = '新增任务';
      document.getElementById('task-id').disabled = false;
      document.getElementById('task-id').value = '';
      document.getElementById('task-name').value = '';
      document.getElementById('task-program').value = '';
      document.getElementById('task-args').value = '';
      document.getElementById('task-workdir').value = '';
      document.getElementById('task-timeout').value = 8;
      document.getElementById('task-autostart').checked = false;
      document.getElementById('task-restart').checked = false;
    }

    async function saveTask() {
      const id = document.getElementById('task-id').value.trim();
      const payload = {
        id: id,
        name: document.getElementById('task-name').value.trim(),
        program: document.getElementById('task-program').value.trim(),
        args: splitArgs(document.getElementById('task-args').value),
        workdir: document.getElementById('task-workdir').value.trim(),
        env: [],
        autostart: document.getElementById('task-autostart').checked,
        restart_on_crash: document.getElementById('task-restart').checked,
        stop_timeout_sec: parseInt(document.getElementById('task-timeout').value || '8', 10)
      };

      const isEdit = !!editingTaskId;
      const url = isEdit ? '/api/tasks/' + editingTaskId : '/api/tasks';
      const method = isEdit ? 'PUT' : 'POST';

      const result = await api(url, {
        method: method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });

      if (result.code !== 0) {
        alert(result.msg || '保存失败');
        return;
      }

      resetForm();
      await loadTasks();
    }

    async function loadTasks() {
      const data = await api('/api/tasks');
      renderTasks(data.data || []);
    }

    async function loadLogs() {
      const taskId = document.getElementById('log-task').value;
      const type = document.getElementById('log-type').value;
      const follow = document.getElementById('log-follow').checked;
      const logsEl = document.getElementById('logs');
      if (!taskId) {
        logsEl.textContent = '';
        return;
      }
      const nearBottom = logsEl.scrollTop + logsEl.clientHeight >= logsEl.scrollHeight - 40;
      const data = await api('/api/tasks/' + taskId + '/logs?type=' + type + '&tail=200');
      logsEl.textContent = (data.data && data.data.content) || '';
      if (follow || nearBottom) {
        logsEl.scrollTop = logsEl.scrollHeight;
      }
    }

    async function clearLogs() {
      const taskId = document.getElementById('log-task').value;
      if (!taskId) {
        return;
      }
      if (!confirm('确认清空当前任务日志吗？')) {
        return;
      }
      const result = await api('/api/tasks/' + taskId + '/clear-logs', { method: 'POST' });
      if (result.code !== 0) {
        alert(result.msg || '清空日志失败');
        return;
      }
      document.getElementById('logs').textContent = '';
      await loadLogs();
    }

    loadTasks();
    setInterval(loadTasks, 5000);
    setInterval(() => {
      if (document.getElementById('log-auto-refresh').checked) {
        loadLogs();
      }
    }, 2000);
  </script>
</body>
</html>`
