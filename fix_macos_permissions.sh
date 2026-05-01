#!/bin/bash
# Fix macOS camera permissions for QR scanning
APP_PATH="$1"

if [ -z "$APP_PATH" ]; then
    echo "Usage: $0 <path-to-Engram.app>"
    exit 1
fi

echo "Adding camera usage descriptions to Info.plist..."
defaults write "$APP_PATH/Contents/Info.plist" NSCameraUsageDescription "Engram needs camera access to scan QR codes"
defaults write "$APP_PATH/Contents/Info.plist" NSMicrophoneUsageDescription "Engram needs microphone access for audio features"
plutil -convert xml1 "$APP_PATH/Contents/Info.plist"

echo "Creating entitlements file..."
cat > /tmp/engram_entitlements.plist << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>com.apple.security.device-camera</key>
    <true/>
    <key>com.apple.security.device-microphone</key>
    <true/>
</dict>
</plist>
EOF

echo "Re-signing app with entitlements..."
codesign --force --deep --sign - --entitlements /tmp/engram_entitlements.plist "$APP_PATH"

echo "Done! App should now have camera access."
