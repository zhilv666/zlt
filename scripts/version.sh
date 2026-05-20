#!/usr/bin/env sh
set -eu

VERSION=$(git -c safe.directory=C:/Users/zhanghoulin/Desktop/tray describe --tags --always | tr -d '\r')
COMMIT=$(git -c safe.directory=C:/Users/zhanghoulin/Desktop/tray rev-parse --short HEAD | tr -d '\r')
BUILD_TIME=$(date +"%Y-%m-%dT%H:%M:%S%z")
BUILT_BY=${USERNAME:-${USER:-unknown}}
PLATFORM="$(uname -s | tr '[:upper:]' '[:lower:]')/$(uname -m)"

printf 'version=%s\n' "$VERSION"
printf 'commit=%s\n' "$COMMIT"
printf 'build_time=%s\n' "$BUILD_TIME"
printf 'built_by=%s\n' "$BUILT_BY"
printf 'platform=%s\n' "$PLATFORM"
