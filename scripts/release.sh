#!/usr/bin/env sh
set -eu

TARGET_OS="${1:-$(go env GOOS)}"
TARGET_ARCH="${2:-$(go env GOARCH)}"

REPO_DIR=$(pwd -W 2>/dev/null || pwd)
VERSION=$(git -c safe.directory="$REPO_DIR" describe --tags --always | tr -d '\r')
COMMIT=$(git -c safe.directory="$REPO_DIR" rev-parse --short HEAD | tr -d '\r')
BUILD_TIME=$(date +"%Y-%m-%dT%H:%M:%S%z")

DIST_DIR="dist"
VERSION_DIR="$DIST_DIR/$VERSION"
BASE_NAME="zlt-$VERSION-$TARGET_OS-$TARGET_ARCH"
WORK_DIR="$VERSION_DIR/.tmp-$BASE_NAME"
LEGACY_DIR="$VERSION_DIR/$BASE_NAME"
BIN_NAME="zlt"
ARTIFACT_PATH="$VERSION_DIR/$BASE_NAME"

case "$TARGET_OS" in
  windows)
    BIN_NAME="zlt.exe"
    ARTIFACT_PATH="$VERSION_DIR/$BASE_NAME.exe"
    ;;
  *)
    ARTIFACT_PATH="$VERSION_DIR/$BASE_NAME.tar.gz"
    ;;
esac

rm -rf "$WORK_DIR" "$LEGACY_DIR"
mkdir -p "$VERSION_DIR" "$WORK_DIR"
sh "./scripts/build.sh" "$WORK_DIR/$BIN_NAME" "$TARGET_OS" "$TARGET_ARCH"

cat > "$VERSION_DIR/build-info.json" <<EOF
{
  "version": "$VERSION",
  "commit": "$COMMIT",
  "build_time": "$BUILD_TIME",
  "os": "$TARGET_OS",
  "arch": "$TARGET_ARCH"
}
EOF

case "$TARGET_OS" in
  windows)
    mv "$WORK_DIR/$BIN_NAME" "$ARTIFACT_PATH"
    ;;
  *)
    go run ./scripts/release_pack.go "$WORK_DIR" "$ARTIFACT_PATH"
    ;;
esac

rm -rf "$WORK_DIR"

printf 'artifact=%s\n' "$ARTIFACT_PATH"
