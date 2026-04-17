#!/bin/bash
# Patch Fyne module cache with QR scanner changes

MODULE_PATH="$HOME/go/pkg/mod/fyne.io/fyne/v2@v2.7.3/internal/driver/mobile/app"
SOURCE_FILE="$(dirname "$0")/vendor/fyne.io/fyne/v2/internal/driver/mobile/app/GoNativeActivity.java"

if [ ! -f "$SOURCE_FILE" ]; then
    echo "Source file not found: $SOURCE_FILE"
    exit 1
fi

if [ ! -d "$MODULE_PATH" ]; then
    echo "Module path not found: $MODULE_PATH"
    exit 1
fi

# Backup original
cp "$MODULE_PATH/GoNativeActivity.java" "$MODULE_PATH/GoNativeActivity.java.bak" 2>/dev/null || true

# Copy patched version
cp "$SOURCE_FILE" "$MODULE_PATH/GoNativeActivity.java"

echo "Patched GoNativeActivity.java in module cache"