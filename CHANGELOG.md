# Changelog

本文件集中记录驻令台各版本的重要变化。后续正式版本发布时，GitHub Actions 会自动将新版本内容插入到文件顶部。

## v0.3.0 (2026-09-10)

### Features

- ✨ feat(app): 新增 zlt auth set 手动设置访问密钥 (b2bcc83)
- ✨ feat(auth): 访问密钥改为保存文本以支持自定义口令 (2d3a17b)
- ✨ feat(web): 网页登录与双列表拖拽排序 (830714f)
- ✨ feat(app): 鉴权装配、密钥命令与排序运行时 (7a2d7a6)
- ✨ feat(api): 鉴权中间件、SSE 失效保护与排序端点 (b9f6c21)
- ✨ feat(auth): 密钥会话与统一鉴权 (13d0df3)

### Fixes

- 🐛 fix(store): 修复保存事务回滚遗漏并支持排序持久化 (e78a4d6)

### Documentation

- 📝 docs(readme): 补充手动设置访问密钥与升级说明 (f6df1e9)
- 📝 docs(cli): 重写 zlt -h 帮助为英文标准排版 (a78c4a6)
- 📝 docs: 同步鉴权、排序、反向代理与升级说明 (a60e7cc)
- 📝 docs(changelog): 更新 v0.2.7 变更日志 [skip ci] (6ef2b83)

## v0.2.7 (2026-07-15)

### Features

- ✨ feat(log): 任务日志按运行归档，支持查看历史 (87b3a2d)
- ✨ feat(web): 日志查看支持一键复制 (94ebba5)
- ✨ feat(settings): 网页可配置日志设置项 (2887fd9)
- ✨ feat(log): 结构化日志、轮转与高效读取 (b86de69)
- ✨ feat(app): 实现应用单实例运行 (028f1a9)

### Documentation

- 📝 docs(changelog): 更新 v0.2.6 变更日志 [skip ci] (8b36902)

## v0.2.6 (2026-07-12)

### Features

- 💄 feat(web): 计划任务立即执行改用 toast 提示 (dffa921)
- 💄 feat(web): 开机自启启用/停用合并为单个切换按钮 (6e2f24a)
- 💄 feat(web): 计划任务前移并新增设置页,优化计划行 (92db40d)
- ✨ feat(web): 新增计划任务管理页面 (fc8833d)
- ✨ feat(scheduler): 新增应用内 Cron 计划任务调度 (3a7d438)

### Documentation

- 📝 docs(readme): 更新界面展示截图 (9adf5f8)
- 📝 docs(release): 统一版本日志与项目文档 (4263b27)
- 📝 docs: 补充 Cron 计划任务说明 (0eacf70)

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
