#!/bin/bash
# DEPRECATED: Use Taskfile.yml (`task build`, `task package-*`) as the canonical build entry.
# This shim is kept for backward compatibility and will be removed. See AGENTS.md.
# Phase 24.4: Mobile Optimizations

echo "Building Engram with size optimization..."
# Build miner from source (instead of bundling pre-compiled binary)
echo "Building miner from source..."
mkdir -p bin
go build -ldflags="-s -w" -o bin/dero-miner-linux-amd64 github.com/deroproject/derohe/cmd/dero-miner
if [ $? -eq 0 ]; then
    echo "Miner build successful!"
    ls -lh bin/dero-miner-linux-amd64
else
    echo "Miner build failed!"
    exit 1
fi

echo ""

# Build with stripped symbols for smaller binary
go build -ldflags="-s -w" -o engram

if [ $? -eq 0 ]; then
    echo ""
    echo "Build successful!"
    echo "Binary size:"
    ls -lh engram | awk '{print "  " $5 " " $9}'
    echo ""
    echo "Optimization flags used:"
    echo "  -s: Omit symbol table and debug info"
    echo "  -w: Omit DWARF symbol table"
else
    echo ""
    echo "Build failed!"
    exit 1
fi

# Android APK build (uncomment to build):
# fyne package -os android -appID "com.derofdn.engram" -icon Icon.png -permissions "android.permission.CAMERA"

# macOS Package
echo "Packaging for macOS..."
fyne package --target darwin --name Engram --app-version 0.7.0 --app-id com.engram.wallet --icon Icon.png --tags migrated_fynedo --executable engram

# Fix macOS camera permissions (required for QR scanning)
scripts/fix_macos_permissions.sh Engram.app
