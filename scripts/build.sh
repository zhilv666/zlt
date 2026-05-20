#!/usr/bin/env sh
set -eu

OUTPUT="${1:-tray.exe}"
TARGET_OS="${2:-$(go env GOOS)}"
TARGET_ARCH="${3:-$(go env GOARCH)}"

REPO_DIR="$(pwd -W 2>/dev/null || pwd)"
PWD_POSIX="$(pwd)"
export GOCACHE="$PWD_POSIX/.gocache"
export GOMODCACHE="$PWD_POSIX/.gomodcache"
export GOSUMDB="off"

VERSION=$(git -c safe.directory="$REPO_DIR" describe --tags --always | tr -d '\r')
COMMIT=$(git -c safe.directory="$REPO_DIR" rev-parse --short HEAD | tr -d '\r')
BUILD_TIME=$(date +"%Y-%m-%dT%H:%M:%S%z")
HOST_OS="$(go env GOOS)"
ICON_PATH="./ico.ico"
RSRC_SYSO="./cmd/tray/rsrc_windows_${TARGET_ARCH}.syso"

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

cleanup() {
  rm -f "$RSRC_SYSO"
}

trap cleanup EXIT INT TERM

if [ "$TARGET_OS" = "windows" ] && [ -f "$ICON_PATH" ]; then
  mkdir -p ./.tools
  if [ "$HOST_OS" = "windows" ] && [ -x "./.tools/rsrc.exe" ]; then
    ./.tools/rsrc.exe -ico "$ICON_PATH" -arch "$TARGET_ARCH" -o "$RSRC_SYSO"
  else
    go run github.com/akavel/rsrc@v0.10.2 -ico "$ICON_PATH" -arch "$TARGET_ARCH" -o "$RSRC_SYSO"
  fi
fi

go build -ldflags "$LDFLAGS" -o "$OUTPUT" ./cmd/tray
