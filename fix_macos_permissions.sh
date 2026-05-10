#!/bin/bash

# fix_macos_permissions.sh
# This script applies necessary macOS permissions and entitlements to the Engram app bundle.
# It should be run on macOS after packaging.

APP_PATH="$1"

if [ -z "$APP_PATH" ]; then
    echo "Usage: $0 <path-to-Engram.app>"
    exit 1
fi

if [ ! -d "$APP_PATH" ]; then
    echo "Error: App bundle not found at $APP_PATH"
    exit 1
fi

echo "Adding usage descriptions to Info.plist..."
# Use plutil to add/replace usage descriptions
plutil -replace NSCameraUsageDescription -string "Engram needs camera access to scan QR codes" "$APP_PATH/Contents/Info.plist"
plutil -replace NSMicrophoneUsageDescription -string "Engram needs microphone access for audio features" "$APP_PATH/Contents/Info.plist"
plutil -replace NSBluetoothAlwaysUsageDescription -string "Engram uses Bluetooth for communication with hardware wallets" "$APP_PATH/Contents/Info.plist"
plutil -convert xml1 "$APP_PATH/Contents/Info.plist"

ENTITLEMENTS_PATH="$(dirname "$0")/Engram.entitlements"

# Fallback entitlements if file is missing
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

echo "Re-signing app with entitlements and hardened runtime..."
# Sign with ad-hoc identity (-) and hardened runtime options
codesign --force --deep --sign - --options runtime --entitlements "$ENTITLEMENTS_PATH" "$APP_PATH"

echo "Done! App should now have camera access and be stable on macOS."
