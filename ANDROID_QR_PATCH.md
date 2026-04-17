// GoNativeActivity.java Patch for QR Code Scanning
// ===============================================
// This patch adds QR code scanning capability to Fyne's GoNativeActivity
// 
// LOCATION: Your Android source at:
//   app/src/main/java/org/golang/app/GoNativeActivity.java
//
// REQUIRED CHANGES:
// 1. Add import for PackageManager (already imported)
// 2. Add constant: private static final int QR_SCAN_CODE = 1001;
// 3. Add native method: private native void sendQRResultNative(String result);
// 4. Add startQRScanner() method
// 5. Add onRequestPermissionsResult() method
// 6. Update onActivityResult() to handle QR_SCAN_CODE
//
// Full modified file with all changes:

/*
package org.golang.app;

import android.app.Activity;
import android.app.NativeActivity;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.graphics.Rect;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.text.Editable;
import android.text.InputType;
import android.text.TextWatcher;
import android.text.method.DigitsKeyListener;
import android.util.Log;
import android.view.Gravity;
import android.view.KeyCharacterMap;
import android.view.View;
import android.view.WindowInsets;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputMethodManager;
import android.view.KeyEvent;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.TextView;
import android.widget.TextView.OnEditorActionListener;

public class GoNativeActivity extends NativeActivity {
    private static GoNativeActivity goNativeActivity;
    private static final int FILE_OPEN_CODE = 1;
    private static final int FILE_SAVE_CODE = 2;
    private static final int QR_SCAN_CODE = 1001;  // <-- ADD THIS

    private static final int DEFAULT_INPUT_TYPE = InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS;

    private static final int DEFAULT_KEYBOARD_CODE = 0;
    private static final int SINGLELINE_KEYBOARD_CODE = 1;
    private static final int NUMBER_KEYBOARD_CODE = 2;
    private static final int PASSWORD_KEYBOARD_CODE = 3;

    // <-- ADD THIS native method declaration
    private native void sendQRResultNative(String result);

    // ... rest of existing code ...

    // === ADD THESE NEW METHODS AT THE END OF THE CLASS ===

    // QR Code scanning support - called from Go via JNI
    public void startQRScanner() {
        Log.i("GoNativeActivity", "startQRScanner called");
        
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                // Check camera permission first
                if (checkSelfPermission(android.Manifest.permission.CAMERA) != PackageManager.PERMISSION_GRANTED) {
                    Log.i("GoNativeActivity", "Camera permission not granted, requesting...");
                    requestPermissions(new String[]{android.Manifest.permission.CAMERA}, 100);
                    Log.i("GoNativeActivity", "Please grant camera permission to scan QR codes");
                    return;
                }
                
                // Permission granted, proceed with scanning using ZXing Intent
                try {
                    Intent intent = new Intent("com.google.zxing.client.android.SCAN");
                    intent.putExtra("SCAN_MODE", "QR_CODE_MODE");
                    intent.setPackage("com.google.zxing.client.android");
                    startActivityForResult(intent, QR_SCAN_CODE);
                } catch (android.content.ActivityNotFoundException e) {
                    Log.e("GoNativeActivity", "No QR scanner app found, opening Play Store...");
                    try {
                        Intent storeIntent = new Intent(Intent.ACTION_VIEW, 
                            Uri.parse("market://details?id=com.google.zxing.client.android"));
                        startActivity(storeIntent);
                    } catch (Exception ex) {
                        Log.e("GoNativeActivity", "Could not open Play Store: " + ex.getMessage());
                    }
                } catch (Exception e) {
                    Log.e("GoNativeActivity", "Error starting QR scanner: " + e.getMessage());
                }
            }
        });
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode == 100) {
            if (grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED) {
                Log.i("GoNativeActivity", "Camera permission granted, starting QR scanner...");
                try {
                    Intent intent = new Intent("com.google.zxing.client.android.SCAN");
                    intent.putExtra("SCAN_MODE", "QR_CODE_MODE");
                    intent.setPackage("com.google.zxing.client.android");
                    startActivityForResult(intent, QR_SCAN_CODE);
                } catch (Exception e) {
                    Log.e("GoNativeActivity", "Error after permission grant: " + e.getMessage());
                }
            } else {
                Log.e("GoNativeActivity", "Camera permission denied");
            }
        }
    }

    // Update existing onActivityResult to handle QR scan:
    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        // Handle QR scan result - ADD THIS BLOCK
        if (requestCode == QR_SCAN_CODE) {
            if (resultCode == Activity.RESULT_OK && data != null) {
                String scanResult = data.getStringExtra("SCAN_RESULT");
                if (scanResult != null) {
                    Log.i("GoNativeActivity", "QR Code scanned: " + scanResult);
                    sendQRResultNative(scanResult);
                }
            } else {
                Log.i("GoNativeActivity", "QR scan cancelled or failed");
            }
            return;
        }

        // ... existing file picker code ...
    }
}
*/

// === ALTERNATIVE: In-app CameraX + ML Kit Solution ===
// For truly self-contained QR scanning without external apps,
// you would need to add CameraX and ML Kit to your Gradle dependencies
// and implement a custom camera preview with ML Kit barcode detection.
// This requires modifying the Android build.gradle files.