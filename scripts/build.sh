#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
"$ROOT/scripts/build-gui.sh"

build_wrapper() {
  echo "Building for $1 $2"
  local windows_file_extension=""
  local extra_ldflags=""
  if [ "$1" == "windows" ]; then
    windows_file_extension=".exe"
    # Build a self-contained Windows binary (no external bitwarden_c.dll).
    extra_ldflags=" -extldflags '-static'"
  fi
  local output="dist/ws-$1-$2$windows_file_extension"
  GOOS=$1 GOARCH=$2 go build -ldflags "-X 'github.com/dont-be-evil-company/withsecrets/internal/lib/version.VERSION=${VERSION}'${extra_ldflags}" -o "$output"
}

build_linux_arm64() {
  build_wrapper "linux" "arm64"
}

build_linux_x86_64() {
  build_wrapper "linux" "amd64"
}

build_linux() {
  build_linux_x86_64
}

build_macos_arm64() {
  build_wrapper "darwin" "arm64"
}

build_macos_x86_64() {
  build_wrapper "darwin" "amd64"
}

build_macos() {
  build_macos_arm64
  build_macos_x86_64
}

build_windows_x86_64() {
  build_wrapper "windows" "amd64"
}

build_windows() {
  build_windows_x86_64
}

case $TARGET_PLATFORM in
  "linux")
    build_linux
    ;;
  "linux-arm64")
    build_linux_arm64
    ;;
  "linux-debug")
    build_linux_debug
    ;;
  "macos")
    build_macos
    ;;
  "windows")
    build_windows
    ;;
  *)
    echo "Error: TARGET_PLATFORM $TARGET_PLATFORM is not supported"
    exit 1
    ;;
esac
