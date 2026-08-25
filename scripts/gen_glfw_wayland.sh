#!/usr/bin/env bash
# Regenerate GLFW Wayland protocol headers from the committed .xml definitions.
#
# go-gl/glfw v3.4 ships the Wayland protocol .xml files under
# glfw/deps/wayland/ but NOT the generated C headers. The Wayland build path
# (c_glfw_lin_wayland.go -> glfw/src/wl_init.c) #includes headers such as
# xdg-shell-client-protocol.h, which must be generated with wayland-scanner.
#
# The default Linux build (no -tags x11/-tags wayland) compiles BOTH the X11
# and Wayland backends, so a missing header breaks `go run .` / `go build .`.
# Re-run this script after `go mod vendor` or a fresh clone to restore them.
#
# CI/Taskfile builds use -tags x11 and do not need these, but generating them
# is harmless and keeps the no-tag local build working.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GLFW_DIR="$REPO_ROOT/vendor/github.com/go-gl/glfw/v3.4/glfw/glfw"
SRC="$GLFW_DIR/src"
DEPS="$GLFW_DIR/deps/wayland"

if ! command -v wayland-scanner >/dev/null 2>&1; then
  echo "wayland-scanner not found; install wayland-protocols (or build with -tags x11)" >&2
  exit 1
fi

if [ ! -d "$DEPS" ]; then
  echo "GLFW wayland deps not found at $DEPS (is vendor populated?)" >&2
  exit 1
fi

mkdir -p "$SRC"
for xml in "$DEPS"/*.xml; do
  [ -e "$xml" ] || continue
  base="$(basename "$xml" .xml)"
  wayland-scanner client-header "$xml" "$SRC/$base-client-protocol.h"
  wayland-scanner private-code "$xml" "$SRC/$base-client-protocol-code.h"
done

echo "Generated GLFW Wayland protocol headers in $SRC"
