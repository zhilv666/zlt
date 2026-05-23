# 驻令台 - Implementation Checklist

## MVP

- [x] Initialize Go project structure
- [x] Add local implementation checklist
- [x] Implement task config model
- [x] Implement task config persistence
- [x] Implement process manager
- [x] Implement process start
- [x] Implement process stop
- [x] Implement runtime status tracking
- [x] Implement log file redirection
- [x] Implement HTTP API server
- [x] Implement static web entry
- [x] Implement tray integration
- [x] Build and verify locally

## Current Scope

Target task:

- `openlist.exe server`

Target capabilities:

- Start from tray
- Stop from tray
- View status from browser
- View logs from browser
- Persist task config locally

## Next Round

- [x] Support task auto-start on app launch
- [x] Support restart-on-crash behavior
- [x] Improve restart flow in web UI
- [x] Add task import/export in the browser
- [x] Add release packaging output
- [x] Add build metadata to release artifacts
- [x] Add cross-platform build tasks
- [x] Verify build and runtime behavior

## Long-Term Plan

- [x] Prepare v0.1.0 release surface
- [x] Add automated tests for core modules
- [x] Harden runtime edge-case handling
- [x] Refine log rotation and retention
- [x] Split embedded web assets into standalone files
- [x] Expand task model with health checks and restart policies
- [x] Implement full cross-platform runtime behavior
- [x] Introduce realtime event streaming for status and logs

## Current Delivery

### 1. Task discovery

- [x] Add task list search and filter
- [x] Improve empty-state hints for filtered results

### 2. Linux headless mode

- [x] Add `run` command for foreground/background service mode
- [x] Add `start` subcommand for daemon/background launch
- [x] Add `stop` subcommand for command-managed task shutdown
- [x] Add `restart` subcommand for command-managed task restart
- [x] Keep web UI available without tray on Linux servers

### 3. Startup automation

- [ ] Add OS startup/auto-launch support
- [ ] Document startup behavior by platform
- [ ] Expose auto-launch controls in the web UI
