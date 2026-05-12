#!/bin/bash
# Patch Fyne module cache with QR scanner changes
# This MUST be run before `fyne package --os android` because the fyne CLI
# compiles Java from the module cache, not the vendor directory.

set -e

MODULE_PATH="$HOME/go/pkg/mod/fyne.io/fyne/v2@v2.7.3/internal/driver/mobile/app"
WV_MODULE_PATH="$HOME/go/pkg/mod/apptrix.org/components@v0.0.0-20260408185842-10f2df6c300c/widget/webview"
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

if [ ! -d "$WV_MODULE_PATH" ]; then
    echo "ERROR: WebView module cache path not found: $WV_MODULE_PATH"
    exit 1
fi

# Make module cache writable (it's read-only by default)
chmod -R u+w "$MODULE_PATH"
chmod -R u+w "$WV_MODULE_PATH"

# Backup originals (only on first run)
for f in GoNativeActivity.java android.c android.go; do
    if [ -f "$MODULE_PATH/$f" ] && [ ! -f "$MODULE_PATH/$f.orig" ]; then
        cp "$MODULE_PATH/$f" "$MODULE_PATH/$f.orig"
        echo "Backed up $f -> $f.orig"
    fi
done

if [ -f "$WV_MODULE_PATH/webview_android.c" ] && [ ! -f "$WV_MODULE_PATH/webview_android.c.orig" ]; then
    cp "$WV_MODULE_PATH/webview_android.c" "$WV_MODULE_PATH/webview_android.c.orig"
    echo "Backed up webview_android.c -> webview_android.c.orig"
fi

if [ -f "$WV_MODULE_PATH/webview_android.h" ] && [ ! -f "$WV_MODULE_PATH/webview_android.h.orig" ]; then
    cp "$WV_MODULE_PATH/webview_android.h" "$WV_MODULE_PATH/webview_android.h.orig"
    echo "Backed up webview_android.h -> webview_android.h.orig"
fi

if [ -f "$WV_MODULE_PATH/webview_android.go" ] && [ ! -f "$WV_MODULE_PATH/webview_android.go.orig" ]; then
    cp "$WV_MODULE_PATH/webview_android.go" "$WV_MODULE_PATH/webview_android.go.orig"
    echo "Backed up webview_android.go -> webview_android.go.orig"
fi

# Copy patched versions
cp "$VENDOR_PATH/GoNativeActivity.java" "$MODULE_PATH/GoNativeActivity.java"
cp "$VENDOR_PATH/android.c"             "$MODULE_PATH/android.c"
cp "$VENDOR_PATH/android.go"            "$MODULE_PATH/android.go"
cp "$VENDOR_PATH/webview_android.c"     "$WV_MODULE_PATH/webview_android.c"
cp "$VENDOR_PATH/webview_android.h"     "$WV_MODULE_PATH/webview_android.h"
cp "$VENDOR_PATH/webview_android.go"    "$WV_MODULE_PATH/webview_android.go"

echo "✅ Patched module cache at: $MODULE_PATH"
echo "✅ Patched WebView module cache at: $WV_MODULE_PATH"
echo ""
echo "Files patched:"
echo "  - GoNativeActivity.java (Camera2 QR scanner Java code + EngramWebViewClient)"
echo "  - android.c             (JNI bridge for startCamera/stopCamera)"
echo "  - android.go            (Go declarations)"
echo "  - webview_android.c     (WebView initialization with custom client)"
echo "  - webview_android.h     (WebView header with hide/show/destroy)"
echo "  - webview_android.go    (WebView Go wrappers for hide/show/destroy)"
echo ""
echo "Now run: fyne package --os android --app-id com.derofdn.engram"