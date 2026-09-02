#!/bin/bash

# fix_macos_permissions.sh
# This script applies necessary macOS permissions and entitlements to the Engram app bundle.
# Works on both macOS (native) and Linux (osxcross cross-compilation).

set -euo pipefail

APP_PATH="${1:?Usage: $0 <path-to-Engram.app>}"

if [ ! -d "$APP_PATH" ]; then
    echo "Error: App bundle not found at $APP_PATH"
    exit 1
fi

PLIST="$APP_PATH/Contents/Info.plist"

if [ ! -f "$PLIST" ]; then
    echo "Error: Info.plist not found at $PLIST"
    exit 1
fi

add_usage_descriptions() {
    local plist="$1"
    echo "Adding usage descriptions to Info.plist..."

    python3 -c "
import plistlib, sys
try:
    with open('$plist', 'rb') as f:
        pl = plistlib.load(f)
    pl['NSCameraUsageDescription'] = 'Engram needs camera access to scan QR codes'
    pl['NSMicrophoneUsageDescription'] = 'Engram needs microphone access for audio features'
    pl['NSBluetoothAlwaysUsageDescription'] = 'Engram uses Bluetooth for communication with hardware wallets'
    with open('$plist', 'wb') as f:
        plistlib.dump(pl, f)
    print('Usage descriptions added successfully')
except Exception as e:
    print(f'Error updating Info.plist: {e}', file=sys.stderr)
    sys.exit(1)
"
}

ENTITLEMENTS_PATH="$(cd "$(dirname "$0")/.." && pwd)/Engram.entitlements"

if [ ! -f "$ENTITLEMENTS_PATH" ]; then
    echo "Warning: Engram.entitlements not found, creating temporary entitlements..."
    cat > "/tmp/engram.entitlements.plist" << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>com.apple.security.device-camera</key><true/>
    <key>com.apple.security.device-microphone</key><true/>
    <key>com.apple.security.network.client</key><true/>
</dict>
</plist>
EOF
    ENTITLEMENTS_PATH="/tmp/engram.entitlements.plist"
fi

SIGN_CMD=""
if [ "$(uname)" = "Darwin" ]; then
    SIGN_CMD="codesign --force --deep --sign - --options runtime --entitlements '$ENTITLEMENTS_PATH' '$APP_PATH'"
elif command -v rcodesign &>/dev/null; then
    SIGN_CMD="rcodesign sign --code-signature-flags runtime -e '$ENTITLEMENTS_PATH' '$APP_PATH'"
fi

if [ -n "$SIGN_CMD" ]; then
    add_usage_descriptions "$PLIST"
    echo "Signing app with entitlements and hardened runtime..."
    eval "$SIGN_CMD"

    if [ "$(uname)" = "Darwin" ]; then
        # Strip quarantine attribute — on Tahoe+ even ad-hoc signed apps
        # are hard-blocked if the quarantine flag is present.
        xattr -dr com.apple.quarantine "$APP_PATH" 2>/dev/null || true
    fi

    echo "Done! App is signed and ready for distribution."
else
    add_usage_descriptions "$PLIST"
    echo ""
    echo "================================================================"
    echo "  WARNING: App bundle is NOT code-signed."
    echo "  macOS Tahoe+ will reject unsigned apps outright."
    echo ""
    echo "  Install rcodesign to sign on Linux:"
    echo "    cargo install apple-codesign --locked"
    echo ""
    echo "  Then re-run this script, or copy the .app to a Mac and run:"
    echo "    codesign --force --deep --sign - --options runtime \\"
    echo "      --entitlements Engram.entitlements \\"
    echo "      $APP_PATH"
    echo "================================================================"
    echo ""
fi
