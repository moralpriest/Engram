#!/bin/bash
# Run this to patch the GoNativeActivity.java in the Fyne module cache with QR scanner support

CACHE_PATH="$HOME/go/pkg/mod/fyne.io/fyne/v2@v2.8.0/internal/driver/mobile/app/GoNativeActivity.java"
SOURCE_PATH="$(dirname "$0")/internal/patches/android/GoNativeActivity.java"

echo "Patching GoNativeActivity.java in module cache..."

# Check if we can write
if cp "$SOURCE_PATH" "$CACHE_PATH" 2>/dev/null; then
    echo "SUCCESS: Patched $CACHE_PATH"
else
    echo "ERROR: Cannot write to cache. You may need to run with elevated permissions or:"
    echo ""
    echo "Option 1: Manually copy the file:"
    echo "  cp $SOURCE_PATH $CACHE_PATH"
    echo ""
    echo "Option 2: If the file is read-only, try:"
    echo "  chmod u+w $CACHE_PATH"
    echo "  cp $SOURCE_PATH $CACHE_PATH"
    echo ""
    echo "Option 3: Delete and re-copy:"
    echo "  rm $CACHE_PATH"
    echo "  cp $SOURCE_PATH $CACHE_PATH"
fi