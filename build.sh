#!/bin/bash
# Build script for Engram with size optimization
# Phase 24.4: Mobile Optimizations

echo "Building Engram with size optimization..."
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
