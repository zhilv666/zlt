# Tray Command Manager

一个基于 Go 的托盘命令管理器。

它负责两件事：

- 托盘中快速启动/停止任务
- 浏览器中管理任务、查看状态、查看日志

当前默认示例任务是：

```text
openlist.exe server
```

## 功能

- 托盘一级菜单启停任务
- 浏览器内增删改查任务
- 查看任务状态、PID、错误信息
- 查看、清空、下载日志
- 任务配置持久化到 SQLite
- 支持自动启动和异常退出自动重启
- 支持任务导入导出
- 支持构建版本信息展示

## 数据目录

运行时数据默认写到：

```text
data/
```

主要内容：

- `data/tasks.db`：任务配置数据库
- `data/logs/<task-id>/`：任务日志

这些内容属于本地运行数据，不需要提交到 Git。

## 本地运行

```sh
go build ./...
```

或者直接构建当前平台产物：

```sh
task build:current
```

程序启动后会打开本地网页控制面板，默认地址：

```text
http://127.0.0.1:3719
```

## 构建命令

```sh
task version
task build:current
task build:windows
task build:linux
task build:darwin
```

构建产物输出到：

```text
bin/
```

## 发布命令

```sh
task release:windows
task release:linux
task release:darwin
```

发布产物输出到按版本区分的目录：

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

## 清理产物

```sh
task clean:dist
```

用于清理历史发布目录。

## 当前限制

- Windows 平台功能最完整
- 非 Windows 平台当前重点是可构建、可运行网页控制面板
- 真正完整的跨平台进程管理仍然是后续事项
