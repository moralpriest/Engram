#!/bin/bash
# Patch Fyne module cache with QR scanner changes
# This MUST be run before `fyne package --os android` because the fyne CLI
# compiles Java from the module cache, not the vendor directory.

set -e

MODULE_PATH="$HOME/go/pkg/mod/fyne.io/fyne/v2@v2.7.4/internal/driver/mobile/app"
VENDOR_PATH="$(dirname "$0")/internal/patches/android"

if [ ! -d "$VENDOR_PATH" ]; then
    echo "ERROR: Vendor path not found: $VENDOR_PATH"
    exit 1
fi

if [ ! -d "$MODULE_PATH" ]; then
    echo "ERROR: Module cache path not found: $MODULE_PATH"
    echo "Run 'go mod download' first."
    exit 1
fi

# Make module cache writable (it's read-only by default)
chmod -R u+w "$MODULE_PATH"

# Backup originals (only on first run)
for f in GoNativeActivity.java android.c android.go; do
    if [ -f "$MODULE_PATH/$f" ] && [ ! -f "$MODULE_PATH/$f.orig" ]; then
        cp "$MODULE_PATH/$f" "$MODULE_PATH/$f.orig"
        echo "Backed up $f -> $f.orig"
    fi
done

# Copy patched versions
cp "$VENDOR_PATH/GoNativeActivity.java" "$MODULE_PATH/GoNativeActivity.java"
cp "$VENDOR_PATH/android.c"             "$MODULE_PATH/android.c"
cp "$VENDOR_PATH/android.go"            "$MODULE_PATH/android.go"

echo "✅ Patched module cache at: $MODULE_PATH"
echo ""
echo "Files patched:"
echo "  - GoNativeActivity.java (Camera2 QR scanner Java code)"
echo "  - android.c             (JNI bridge for startCamera/stopCamera)"
echo "  - android.go            (Go declarations)"
echo ""
echo "Now run: fyne package --os android --app-id com.derofdn.engram"