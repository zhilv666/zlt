# 驻令台 / Tray Command Manager

> 一个把“托盘快速启停”和“浏览器可视化管理”结合在一起的本地任务管理器。

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)
![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-4E5EE4)
![Storage](https://img.shields.io/badge/Storage-SQLite-003B57?logo=sqlite)
![UI](https://img.shields.io/badge/UI-Tray%20%2B%20Web-111827)

驻令台适合这类本地服务管理场景：

- 把 `openlist.exe server`、`gitea web`、脚本服务等常驻任务挂到托盘
- 不想每次都手敲命令，但又需要保留工作目录、参数、日志和重启策略
- 希望多个本地服务统一管理，而不是每个都单独放一个快捷方式

它把职责拆成两层：

- 托盘：负责快速启动、停止、查看版本
- 浏览器：负责管理任务、查看状态、查看日志、调整策略

默认示例任务：

```text
openlist.exe server
```

## 为什么用它

- 不改原命令：直接托管现有命令和工作目录，不需要改造服务程序
- 控制与管理分离：托盘做快操作，网页做细操作，使用成本低
- 面向多任务：支持分页、日志筛选、实时状态推送，不会一多就乱
- 本地优先：配置落地到 SQLite，日志落地到本地文件，不依赖外部服务
- 发布友好：Windows GUI 构建、版本注入、图标注入和版本化发布都已经打通

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

最短路径：

```sh
task build:current
./bin/tray-current
```

Windows GUI 版：

```sh
task build:windows
./bin/tray-windows-amd64.exe
```

程序启动后：

- Windows 会常驻托盘
- 浏览器控制面板地址默认是 `http://127.0.0.1:3719`
- 首次启动会自动创建 `data/tasks.db` 和日志目录

如果你只想快速体验，可以先添加一个最简单的任务：

```text
ID: openlist
Name: OpenList
Program: openlist.exe
Args: server
WorkDir: D:\SoftWare\OpenList
```

## 适合管理什么

- 本地 Web 服务
- 文件同步、备份、代理、开发辅助服务
- 带健康检查地址的 HTTP 服务
- 需要自动拉起、异常重启、日志追踪的命令行程序

不太适合：

- 需要复杂依赖编排的集群型服务
- 强交互式前台程序
- 需要严格权限隔离和多用户调度的服务编排场景

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

版本信息会注入到：

- 托盘菜单中的 `Version`
- 网页底部版本显示
- 发布目录中的 `build-info.json`

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

## 当前能力边界

- 托盘完整体验目前以 Windows 为主
- Linux / macOS 侧重点是运行时控制和浏览器面板
- 当前是单机本地任务管理器，不是远程节点调度器

## 开发说明

当前仓库约定：

- `web/` 在根目录，不再塞回 `internal/`
- `data/` 是本地运行数据目录，不提交
- `bin/`、`dist/`、`.gocache/`、`.gomodcache/`、`.tools/` 都属于构建产物或本地缓存

测试命令：

```sh
go test ./...
```
