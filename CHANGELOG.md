# Changelog

本文件集中记录驻令台各版本的重要变化。后续正式版本发布时，GitHub Actions 会自动将新版本内容插入到文件顶部。

## v0.2.5 (2026-07-11)

### Features

- 更新网页界面与应用图标，改善整体视觉体验。

## v0.2.4 (2026-07-09)

### Features

- 支持为任务配置环境变量。

### Fixes

- 修复任务日志中文乱码问题。
- 阻止在 `sudo` 环境下错误执行用户级开机自启操作。

### Maintenance

- 改用 Git 提交记录自动生成 Release Notes 和 `CHANGELOG.md`。
- 完善发布工作流，使发布说明与变更日志保持一致。

## v0.2.3 (2026-06-12)

### Fixes

- 修复 Windows 开机自启的工作目录和注册表写入问题。

### Documentation

- 添加 GitHub Changelog 配置。

## v0.2.2 (2026-05-24)

### Features

- 完善 Windows、Linux、macOS 跨平台运行与发布能力。
- 新增 Linux 无界面服务模式及 `start`、`stop`、`restart`、`status` 命令。
- 新增 Linux 命令行和网页端软件开机自启管理。
- 新增任务搜索、日志管理、健康检查和异常重启能力。
- 统一项目名称为“驻令台”，可执行文件名称为 `zlt`。
- 将任务配置持久化迁移至 SQLite。
- 增加构建版本信息、托盘操作和跨平台构建产物。

### Fixes

- 修复 Release 工作流仓库上下文。
- 修复 Windows 子进程窗口显示问题。

### Maintenance

- 添加 GitHub Actions CI、自动构建和 Release 发布流程。
- 补充核心模块自动化测试与发布文档。
- 清理遗留数据文件和临时页面脚本。

## v0.0.1 (2026-05-19)

### Features

- 初始化驻令台托盘命令管理器。
- 支持本地任务配置、进程启停、日志重定向、HTTP API 和网页管理入口。
