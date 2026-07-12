# 驻令台

> 把“托盘快速启停”和“浏览器精细管理”合在一起的本地命令管理器。

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)
![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-4E5EE4)
![Storage](https://img.shields.io/badge/Storage-SQLite-003B57?logo=sqlite)
![UI](https://img.shields.io/badge/UI-System%20Tray%20%2B%20Web-111827)
![License](https://img.shields.io/badge/License-GPL--3.0-blue.svg)

驻令台用于统一管理本地脚本、守护进程和辅助服务。托盘负责快速启停，浏览器面板负责任务配置、状态观察、计划调度和日志查看。

## 功能特性

- 托盘菜单快速启动、停止任务
- 浏览器管理任务的增删改查
- SQLite 持久化任务和计划配置
- 任务自动启动、异常重启和健康检查
- Cron 计划任务定时启动、停止或重启任务
- 任务日志与系统日志查看、下载和清理
- ANSI 彩色日志渲染
- 任务搜索、筛选和分页
- Windows 软件开机自启与 Linux 无界面运行
- Windows、Linux、macOS 跨平台构建与发布

## 快速开始

```sh
task build:current
./bin/zlt-current
```

Windows GUI 版：

```sh
./bin/zlt-windows-amd64.exe
```

默认控制面板地址：`http://127.0.0.1:3719`

> 每个版本的更新内容统一记录在 [CHANGELOG.md](./CHANGELOG.md)。

<details>
<summary>项目命名与设计理念</summary>

- 项目名：`驻令台`
- 仓库名：`zhulingtai`
- 可执行文件：`zlt`

“驻”表示常驻运行与后台守护，“令”表示命令、脚本和服务进程，“台”表示统一管理入口。驻令台不是单纯的命令启动器，而是面向本地服务管理的驻留控制台。

</details>

<details>
<summary>项目结构</summary>

```text
.
├── cmd/zlt/             # CLI / GUI 入口
├── internal/
│   ├── api/             # HTTP API
│   ├── app/             # 运行时、托盘、CLI、自启动
│   ├── process/         # 进程管理
│   ├── scheduler/       # Cron 计划任务调度器
│   ├── store/           # SQLite 持久化
│   └── task/            # 任务与计划模型
├── scripts/             # 构建与发布脚本
├── web/                 # 内嵌网页面板
├── ico.ico              # Windows 图标
├── Taskfile.yml         # 常用任务
└── embed_assets.go      # 静态资源嵌入
```

</details>

<details>
<summary>构建与运行命令</summary>

构建：

```sh
task build:current
task build:windows
task build:linux
task build:darwin
```

查看版本：

```sh
./bin/zlt-current version
```

Linux 无界面模式：

```sh
./bin/zlt-linux-amd64 run
./bin/zlt-linux-amd64 start
./bin/zlt-linux-amd64 status
./bin/zlt-linux-amd64 stop
./bin/zlt-linux-amd64 restart
```

指定监听地址：

```sh
./bin/zlt-linux-amd64 run --addr 0.0.0.0:3719
./bin/zlt-linux-amd64 start --addr 0.0.0.0:3719
```

Linux 软件开机自启：

```sh
./bin/zlt-linux-amd64 autostart enable
./bin/zlt-linux-amd64 autostart status
./bin/zlt-linux-amd64 autostart disable
```

Windows 可在网页“设置”页面查看、启用或停用软件开机自启。

</details>

<details>
<summary>任务配置示例</summary>

```json
{
  "id": "openlist",
  "name": "OpenList",
  "program": "openlist.exe",
  "args": ["server"],
  "workdir": "D:/SoftWare/OpenList",
  "env": [],
  "autostart": false,
  "restart_on_crash": false,
  "stop_timeout_sec": 8,
  "restart_delay_sec": 2,
  "max_restart_count": 0,
  "health_check_url": "",
  "health_check_interval_sec": 0,
  "health_check_failure_threshold": 0
}
```

Python 虚拟环境建议直接填写解释器路径，不需要提前执行 `activate`：

```json
{
  "id": "python-service",
  "name": "Python Service",
  "program": "/home/user/project/.venv/bin/python",
  "args": ["main.py"],
  "workdir": "/home/user/project",
  "env": []
}
```

</details>

<details>
<summary>Cron 计划任务</summary>

网页“计划任务”页面可为已登记任务配置定时动作：

- 动作：`start`、`stop`、`restart`
- 表达式：标准 5 段 Cron，例如 `0 8 * * 1-5`
- 时区：默认跟随系统，也可指定 `Asia/Shanghai` 等 IANA 时区
- 支持启用、停用、立即执行、下次执行时间和最近执行结果

执行规则：

- 已运行任务再次执行 `start` 时跳过
- 已停止任务再次执行 `stop` 时跳过
- 同一计划上一次未结束时跳过重复触发
- 非法 Cron 表达式或时区无法保存
- 存在关联计划的任务不能直接删除
- 驻令台未运行时，进程内计划不会触发

API：

```text
GET    /api/schedules
POST   /api/schedules
PUT    /api/schedules/{id}
DELETE /api/schedules/{id}
POST   /api/schedules/{id}/enable
POST   /api/schedules/{id}/disable
POST   /api/schedules/{id}/run
```

</details>

<details>
<summary>日志说明</summary>

- 任务日志写入 `data/logs/<task_id>/app.log`
- 旧版 `stdout.log` 和 `stderr.log` 仍可兼容读取
- 系统日志写入 `data/app.log`
- 网页支持 ANSI 彩色渲染、纯文本切换、下载和清理

</details>

<details>
<summary>界面截图</summary>

任务列表：

![任务列表](assets/image-20260524172150165.png)

日志查看：

![日志查看](assets/image-20260524172229289.png)

Windows 托盘菜单：

![Windows 托盘菜单](assets/image-20260524172325815.png)

Linux 无界面运行：

![Linux 无界面运行](assets/image-20260524174837383.png)

</details>

<details>
<summary>发布与开发</summary>

环境要求：Go 1.25+，推荐安装 `task`。

```sh
task version
task release:windows
task release:linux
task release:darwin
go test ./...
```

推送 `v*` 标签后，GitHub Actions 会自动构建 Windows、Linux、macOS 产物，生成 Release Notes，并更新 [CHANGELOG.md](./CHANGELOG.md)。

```sh
git tag v0.2.6
git push origin main --tags
```

发布产物位于 `dist/<version>/`。Windows 使用 `.exe`，Linux 和 macOS 使用 `.tar.gz`，并提供 `SHA256SUMS.txt`。

开发约定：

- `ico.ico`、`web/` 固定保留在仓库根目录
- `data/` 为本地运行数据，不参与提交
- `bin/`、`dist/`、`.gocache/`、`.gomodcache/`、`.gotmp/`、`.tools/` 为构建产物或缓存

</details>

## 社区支持

<div align="center">

**学 AI，上 L 站**

[![LINUX DO](https://img.shields.io/badge/LINUX%20DO-社区-gray?style=flat-square)](https://linux.do/) [![社区支持](https://img.shields.io/badge/社区支持-交流-blue?style=flat-square)](https://linux.do/)

本项目在 [LINUX DO](https://linux.do/) 社区发布与交流，感谢佬友们的支持与反馈。

</div>

## 许可证

GPL-3.0 License. See [LICENSE](./LICENSE).
