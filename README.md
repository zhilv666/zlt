# 驻令台

> 一个把“托盘快速启停”和“浏览器精细管理”合在一起的本地命令管理器。

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)
![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-4E5EE4)
![Storage](https://img.shields.io/badge/Storage-SQLite-003B57?logo=sqlite)
![UI](https://img.shields.io/badge/UI-Tray%20%2B%20Web-111827)

驻令台适合管理这些本地命令：

- `openlist.exe server`
- `gitea web`
- 各类常驻脚本和辅助服务

它把职责分成两层：

- 托盘：一键启动、停止、查看版本
- 浏览器：任务增删改查、日志查看、状态管理

## 核心能力

- 托盘菜单快速启停任务
- 网页端管理任务和日志
- 任务列表分页展示
- 日志支持筛选、自动刷新和滚动查看
- 支持自动启动、崩溃重启和健康检查
- 任务配置持久化到 SQLite
- 构建信息注入到版本和发布产物
- Windows 构建自动注入图标

## 快速开始

```sh
task build:current
./bin/tray-current
```

Windows GUI 版：

```sh
task build:windows
./bin/tray-windows-amd64.exe
```

启动后默认访问：

```text
http://127.0.0.1:3719
```

示例任务：

```text
ID: openlist
Name: OpenList
Program: openlist.exe
Args: server
WorkDir: D:\SoftWare\OpenList
```

## 任务模型

每个任务至少包含：

- `id`
- `name`
- `program`
- `args`
- `workdir`

常用行为字段：

- `autostart`
- `restart_on_crash`
- `stop_timeout_sec`
- `restart_delay_sec`
- `max_restart_count`
- `health_check_url`
- `health_check_interval_sec`
- `health_check_failure_threshold`

## 构建与发布

```sh
task version
task build:current
task release:windows
task release:linux
task release:darwin
```

产物目录：

```text
bin/
dist/<version>/
```

## 目录约定

- `web/` 保持在仓库根目录
- `ico.ico` 保持在仓库根目录
- `data/` 是本地运行数据，不提交
- `bin/`、`dist/`、`.gocache/`、`.gomodcache/`、`.tools/` 都属于构建产物或本地缓存

测试命令：

```sh
go test ./...
```
