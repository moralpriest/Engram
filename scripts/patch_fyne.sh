#!/bin/bash
# Patch Fyne module cache with QR scanner changes
# This MUST be run before `fyne package --os android` because the fyne CLI
# compiles Java from the module cache, not the vendor directory.

set -e

MODULE_PATH="$HOME/go/pkg/mod/fyne.io/fyne/v2@v2.8.1/internal/driver/mobile/app"
VENDOR_PATH="$(cd "$(dirname "$0")/.." && pwd)/internal/patches/android"

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
for f in GoNativeActivity.java XSWDForegroundService.java android.c android.go; do
    if [ -f "$MODULE_PATH/$f" ] && [ ! -f "$MODULE_PATH/$f.orig" ]; then
        cp "$MODULE_PATH/$f" "$MODULE_PATH/$f.orig"
        echo "Backed up $f -> $f.orig"
    fi
done

# Copy patched versions
cp "$VENDOR_PATH/GoNativeActivity.java"       "$MODULE_PATH/GoNativeActivity.java"
cp "$VENDOR_PATH/XSWDForegroundService.java"  "$MODULE_PATH/XSWDForegroundService.java"
cp "$VENDOR_PATH/android.c"                   "$MODULE_PATH/android.c"
cp "$VENDOR_PATH/android.go"                  "$MODULE_PATH/android.go"
# FyneNotificationReceiver.java is a pristine upstream 2.8.1 file; keep it in
# the module cache too so the javac step in build_fyne_custom.sh stays
# self-contained when the fyne CLI compiles from the patches dir.
cp "$VENDOR_PATH/FyneNotificationReceiver.java" "$MODULE_PATH/FyneNotificationReceiver.java"

# Restore the mobile AppTabs label size. fyne 2.8.x renders tab text at
# caption size (theme.SizeNameCaptionText) on mobile, shrinking it from the
# normal text size (15px -> 11px with Engram's theme). Removing that branch
# restores the pre-2.8.0 behavior where tabs always use SizeNameText.
TABS_DIR="$HOME/go/pkg/mod/fyne.io/fyne/v2@v2.8.1/container"
TABS_FILE="$TABS_DIR/tabs.go"
if [ -f "$TABS_FILE" ]; then
    chmod -R u+w "$TABS_DIR"
    if [ ! -f "$TABS_FILE.orig" ]; then
        cp "$TABS_FILE" "$TABS_FILE.orig"
        echo "Backed up tabs.go -> tabs.go.orig"
    fi
    perl -0pi -e 's/\tif isMobile\(r\.button\.tabs\) \{\n\t\tr\.label\.TextSize = th\.Size\(theme\.SizeNameCaptionText\)\n\t\}\n//g' "$TABS_FILE"
    echo "Patched container/tabs.go (mobile tab text size)"
fi

echo "✅ Patched module cache at: $MODULE_PATH"
echo ""
echo "Files patched:"
echo "  - GoNativeActivity.java       (Camera2 QR scanner, XSWD service Java code)"
echo "  - XSWDForegroundService.java  (Android foreground service)"
echo "  - android.c                   (JNI bridge for startCamera/stopCamera, XSWD service)"
echo "  - android.go                  (Go declarations)"
echo "  - container/tabs.go           (mobile tab text size)"
echo ""
echo "Now run: fyne package --os android --app-id com.derofdn.engram"