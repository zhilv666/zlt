#!/usr/bin/env sh
set -eu

OUTPUT="${1:-tray.exe}"
TARGET_OS="${2:-$(go env GOOS)}"
TARGET_ARCH="${3:-$(go env GOARCH)}"

REPO_DIR=$(pwd -W 2>/dev/null || pwd)
export GOCACHE="$REPO_DIR/.gocache"
export GOMODCACHE="$REPO_DIR/.gomodcache"
export GOSUMDB="off"

VERSION=$(git -c safe.directory="$REPO_DIR" describe --tags --always | tr -d '\r')
COMMIT=$(git -c safe.directory="$REPO_DIR" rev-parse --short HEAD | tr -d '\r')
BUILD_TIME=$(date +"%Y-%m-%dT%H:%M:%S%z")

export GOOS="$TARGET_OS"
export GOARCH="$TARGET_ARCH"

LDFLAGS="-s -w \
-X tray/internal/buildinfo.Version=$VERSION \
-X tray/internal/buildinfo.Commit=$COMMIT \
-X tray/internal/buildinfo.BuildTime=$BUILD_TIME \
-X tray/internal/buildinfo.TargetOS=$TARGET_OS \
-X tray/internal/buildinfo.TargetArch=$TARGET_ARCH"

if [ "$TARGET_OS" = "windows" ]; then
  LDFLAGS="-H windowsgui $LDFLAGS"
fi

case "$TARGET_OS" in
  windows)
    case "$OUTPUT" in
      *.exe) ;;
      *) OUTPUT="${OUTPUT}.exe" ;;
    esac
    ;;
esac

printf 'version=%s\n' "$VERSION"
printf 'commit=%s\n' "$COMMIT"
printf 'build_time=%s\n' "$BUILD_TIME"
printf 'platform=%s/%s\n' "$TARGET_OS" "$TARGET_ARCH"

go build -ldflags "$LDFLAGS" -o "$OUTPUT" ./cmd/tray
