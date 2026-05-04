#!/bin/bash
# Fix macOS camera permissions and stability for QR scanning
APP_PATH="$1"

if [ -z "$APP_PATH" ]; then
    echo "Usage: $0 <path-to-Engram.app>"
    exit 1
fi

echo "Adding usage descriptions to Info.plist..."
# Use plutil for more robust Info.plist modification
plutil -replace NSCameraUsageDescription -string "Engram needs camera access to scan QR codes for wallet authentication" "$APP_PATH/Contents/Info.plist"
plutil -replace NSMicrophoneUsageDescription -string "Engram needs microphone access for audio features" "$APP_PATH/Contents/Info.plist"
plutil -replace NSBluetoothAlwaysUsageDescription -string "Engram uses Bluetooth for communication with hardware wallets" "$APP_PATH/Contents/Info.plist"
plutil -convert xml1 "$APP_PATH/Contents/Info.plist"

ENTITLEMENTS_PATH="$(dirname "$0")/Engram.entitlements"
if [ ! -f "$ENTITLEMENTS_PATH" ]; then
    echo "Error: Entitlements file not found at $ENTITLEMENTS_PATH"
    exit 1
fi

echo "Re-signing app with entitlements and hardened runtime..."
# Use --options runtime for hardened runtime compatibility on modern macOS
codesign --force --deep --sign - --options runtime --entitlements "$ENTITLEMENTS_PATH" "$APP_PATH"

echo "Done! App should now be stable and have camera access."
