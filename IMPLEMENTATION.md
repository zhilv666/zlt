# Tray Command Manager - Implementation Checklist

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
