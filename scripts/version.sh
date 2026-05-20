#!/usr/bin/env sh
set -eu

REPO_DIR=$(pwd -W 2>/dev/null || pwd)

VERSION=$(git -c safe.directory="$REPO_DIR" describe --tags --always | tr -d '\r')
COMMIT=$(git -c safe.directory="$REPO_DIR" rev-parse --short HEAD | tr -d '\r')
BUILD_TIME=$(date +"%Y-%m-%dT%H:%M:%S%z")
PLATFORM="$(uname -s | tr '[:upper:]' '[:lower:]')/$(uname -m)"

printf 'version=%s\n' "$VERSION"
printf 'commit=%s\n' "$COMMIT"
printf 'build_time=%s\n' "$BUILD_TIME"
printf 'platform=%s\n' "$PLATFORM"
