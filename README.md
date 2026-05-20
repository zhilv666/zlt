# 驻令台 / Tray Command Manager

一个基于 Go 的本地托盘命令管理器。

它把“常驻托盘的快速启停”和“浏览器里的任务管理”拆开处理：

- 托盘：负责快速启动、停止、查看版本
- 浏览器：负责管理任务、查看状态、查看日志、调整策略

适合这类场景：

- 把 `openlist.exe server`、`gitea web`、脚本服务等常驻任务挂到托盘
- 不想每次都手敲命令，但又需要保留工作目录、参数、日志和重启策略
- 希望多个本地服务统一管理，而不是每个都单独放一个快捷方式

默认示例任务：

```text
openlist.exe server
```

## 功能概览

- 托盘菜单中快速启动/停止任务
- 托盘菜单显示当前版本
- 浏览器中增删改查任务
- 任务列表分页展示，适合多任务场景
- 日志页支持任务筛选、实时推送、下载与清空
- 实时推送任务状态和日志内容
- 记录任务 PID、退出状态、错误信息
- 支持自动启动、异常退出自动重启、重启延迟、最大重启次数
- 支持 HTTP 健康检查与失败阈值控制
- 任务配置持久化到 SQLite
- 支持任务导入导出
- 支持构建版本信息注入
- Windows 构建产物自动注入 `exe` 图标

## 快速开始

1. 构建当前平台可执行文件：

```sh
task build:current
```

2. 启动程序：

```sh
./bin/tray-current
```

Windows 下也可以直接构建 GUI 版：

```sh
task build:windows
./bin/tray-windows-amd64.exe
```

3. 程序启动后：

- Windows 会常驻托盘
- 浏览器控制面板地址默认是 `http://127.0.0.1:3719`
- 首次启动会自动创建 `data/tasks.db` 和日志目录

## 任务模型

每个任务至少包含这几个核心字段：

- `id`：唯一标识
- `name`：显示名称
- `program`：可执行文件或命令
- `args`：参数数组
- `workdir`：工作目录

当前支持的主要行为字段：

- `autostart`：程序启动时自动拉起任务
- `restart_on_crash`：异常退出后自动重启
- `stop_timeout_sec`：停止等待超时
- `restart_delay_sec`：自动重启延迟
- `max_restart_count`：最大自动重启次数，`0` 表示不限制
- `health_check_url`：HTTP 健康检查地址
- `health_check_interval_sec`：健康检查间隔
- `health_check_failure_threshold`：连续失败阈值

一个典型任务示例：

```json
{
  "id": "openlist",
  "name": "OpenList",
  "program": "openlist.exe",
  "args": ["server"],
  "workdir": "D:/SoftWare/OpenList",
  "env": [],
  "autostart": true,
  "restart_on_crash": true,
  "stop_timeout_sec": 8,
  "restart_delay_sec": 2,
  "max_restart_count": 0,
  "health_check_url": "http://127.0.0.1:5244/health",
  "health_check_interval_sec": 10,
  "health_check_failure_threshold": 3
}
```

## 运行时数据

运行时数据默认写入：

```text
data/
```

主要内容：

- `data/tasks.db`：任务配置数据库
- `data/logs/<task-id>/stdout.log`：标准输出日志
- `data/logs/<task-id>/stderr.log`：标准错误日志

日志会在任务启动前按大小自动轮转：

- 当前日志：`stdout.log` / `stderr.log`
- 历史日志：`stdout.log.1` ~ `stdout.log.3`
- 单文件超过约 `10MB` 时触发轮转

这些内容属于本地运行数据，不需要提交到 Git。仓库默认已忽略 `data/`。

## 构建与发布

常用命令：

```sh
task version
task build:current
task build:windows
task build:linux
task build:darwin
task release:windows
task release:linux
task release:darwin
```

构建产物输出到：

```text
bin/
```

发布产物输出到：

```text
dist/<version>/
```

例如：

```text
dist/
└── v0.0.3-1-g000fbdb/
    ├── build-info.json
    ├── tray-v0.0.3-1-g000fbdb-linux-amd64.tar.gz
    └── tray-v0.0.3-1-g000fbdb-windows-amd64.exe
```

清理历史发布目录：

```sh
task clean:dist
```

## 图标说明

- 根目录的 `ico.ico` 会同时用于：
  - 托盘运行时图标
  - Windows `exe` 文件资源图标
- Windows 构建时会自动生成临时 `.syso` 资源文件并在构建后清理
- 本地图标工具放在 `./.tools/`，不会污染系统目录

## 目录结构

```text
.
├── cmd/tray/            # 程序入口
├── internal/
│   ├── api/             # HTTP API 与 SSE
│   ├── app/             # 应用启动与托盘控制
│   ├── buildinfo/       # 版本与构建信息
│   ├── process/         # 进程管理与平台差异实现
│   ├── store/           # SQLite 持久化
│   └── task/            # 任务模型
├── scripts/             # 构建与发布脚本
├── web/                 # 前端页面
├── ico.ico              # Windows / 托盘图标源文件
├── embed_assets.go      # 根级静态资源嵌入入口
├── Taskfile.yml         # 常用任务命令
└── IMPLEMENTATION.md    # 实现清单
```

说明：

- `web/` 保持在项目根目录，便于直接修改和嵌入
- `ico.ico` 保持在项目根目录，构建脚本与托盘直接复用
- 根目录只保留一个 `embed_assets.go`，避免多个零散 embed 文件

## 平台说明

- Windows：托盘控制 + 浏览器控制面板 + 任务进程管理
- Linux / macOS：浏览器控制面板 + 任务进程管理
- 非 Windows 平台使用独立进程组启动和停止任务，避免只杀主进程、不清理子进程

## 开发说明

当前仓库约定：

- `web/` 在根目录，不再塞回 `internal/`
- `data/` 是本地运行数据目录，不提交
- `bin/`、`dist/`、`.gocache/`、`.gomodcache/`、`.tools/` 都属于构建产物或本地缓存

测试命令：

```sh
go test ./...
```
