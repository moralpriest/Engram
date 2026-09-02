#!/usr/bin/env bash
# Cross-package Engram for macOS from Linux using osxcross.
# Usage: ./build_macos.sh <arm64|amd64> -- fyne package <args...>
# (the -- is optional; any args after the arch are passed to `fyne package`)
set -euo pipefail

OX="${OSXCROSS_DIR:-${HOME}/osxcross/target}"
export PATH="$OX/bin:$PATH"
export SDKROOT="$OX/SDK/MacOSX14.5.sdk"
export MACOSX_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-10.11}"
export CGO_ENABLED=1
export GOOS=darwin

ARCH="${1:?usage: ./build_macos.sh <arm64|amd64> [--] fyne package args...}"
shift
[ "$1" = "--" ] && shift

case "$ARCH" in
  arm64) TRIPLE="aarch64-apple-darwin23.5";;
  amd64) TRIPLE="x86_64-apple-darwin23.5";;
  *) echo "unsupported arch: $ARCH (expected arm64|amd64)" >&2; exit 1;;
esac

export GOARCH="$ARCH"
export CC="$OX/bin/$TRIPLE-clang"
export CXX="$OX/bin/$TRIPLE-clang++"

exec fyne package --target darwin "$@"