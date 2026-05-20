#!/usr/bin/env sh
set -eu

OUTPUT="${1:-tray.exe}"
TARGET_OS="${2:-${GOOS:-windows}}"
TARGET_ARCH="${3:-${GOARCH:-amd64}}"

REPO_DIR=$(pwd -W 2>/dev/null || pwd)
export GOCACHE="$REPO_DIR/.gocache"
export GOMODCACHE="$REPO_DIR/.gomodcache"
export GOSUMDB="off"

VERSION=$(git -c safe.directory=C:/Users/zhanghoulin/Desktop/tray describe --tags --always | tr -d '\r')
COMMIT=$(git -c safe.directory=C:/Users/zhanghoulin/Desktop/tray rev-parse --short HEAD | tr -d '\r')
BUILD_TIME=$(date +"%Y-%m-%dT%H:%M:%S%z")

export GOOS="$TARGET_OS"
export GOARCH="$TARGET_ARCH"

LDFLAGS="-X tray/internal/buildinfo.Version=$VERSION \
-X tray/internal/buildinfo.Commit=$COMMIT \
-X tray/internal/buildinfo.BuildTime=$BUILD_TIME \
-X tray/internal/buildinfo.TargetOS=$TARGET_OS \
-X tray/internal/buildinfo.TargetArch=$TARGET_ARCH"

printf 'version=%s\n' "$VERSION"
printf 'commit=%s\n' "$COMMIT"
printf 'build_time=%s\n' "$BUILD_TIME"
printf 'platform=%s/%s\n' "$TARGET_OS" "$TARGET_ARCH"

go build -ldflags "$LDFLAGS" -o "$OUTPUT" ./cmd/tray
