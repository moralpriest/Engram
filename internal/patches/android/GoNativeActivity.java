package org.golang.app;

import android.app.Activity;
import android.app.NativeActivity;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.graphics.ImageFormat;
import android.graphics.Rect;
import android.hardware.camera2.CameraAccessException;
import android.hardware.camera2.CameraCaptureSession;
import android.hardware.camera2.CameraCharacteristics;
import android.hardware.camera2.CameraDevice;
import android.hardware.camera2.CameraManager;
import android.hardware.camera2.CaptureRequest;
import android.hardware.camera2.params.StreamConfigurationMap;
import android.media.Image;
import android.media.ImageReader;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.HandlerThread;
import android.text.Editable;
import android.text.InputType;
import android.text.TextWatcher;
import android.text.method.DigitsKeyListener;
import android.util.Log;
import android.util.Size;
import android.view.Gravity;
import android.view.KeyCharacterMap;
import android.view.Surface;
import android.view.View;
import android.view.WindowInsets;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputMethodManager;
import android.view.KeyEvent;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.TextView;
import android.widget.TextView.OnEditorActionListener;

import java.nio.ByteBuffer;
import java.util.Arrays;
import java.util.Collections;

public class GoNativeActivity extends NativeActivity {
	private static GoNativeActivity goNativeActivity;
	private static final int FILE_OPEN_CODE = 1;
	private static final int FILE_SAVE_CODE = 2;
	private static final int CAMERA_PERMISSION_CODE = 100;

	private static final int DEFAULT_INPUT_TYPE = InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS;

	private static final int DEFAULT_KEYBOARD_CODE = 0;
	private static final int SINGLELINE_KEYBOARD_CODE = 1;
	private static final int NUMBER_KEYBOARD_CODE = 2;
	private static final int PASSWORD_KEYBOARD_CODE = 3;

    private native void filePickerReturned(String str);
    private native void insetsChanged(int top, int bottom, int left, int right);
    private native void keyboardTyped(String str);
    private native void keyboardDelete();
    private native void backPressed();
    private native void setDarkMode(boolean dark);
    private static native void cameraFrameAvailable(byte[] data, int width, int height);
    private static native void cameraError(String message);

	private EditText mTextEdit;
	private boolean ignoreKey = false;
	private boolean keyboardUp = false;

	// Camera2 QR scanner fields
	private CameraDevice mCameraDevice;
	private CameraCaptureSession mCaptureSession;
	private ImageReader mImageReader;
	private HandlerThread mCameraThread;
	private Handler mCameraHandler;
	private boolean mCameraRunning = false;

	public GoNativeActivity() {
		super();
		goNativeActivity = this;
	}

	String getTmpdir() {
		return getCacheDir().getAbsolutePath();
	}

	void updateLayout() {
	    try {
            WindowInsets insets = getWindow().getDecorView().getRootWindowInsets();
            if (insets == null) {
                return;
            }

            insetsChanged(insets.getSystemWindowInsetTop(), insets.getSystemWindowInsetBottom(),
                insets.getSystemWindowInsetLeft(), insets.getSystemWindowInsetRight());
        } catch (java.lang.NoSuchMethodError e) {
    	    Rect insets = new Rect();
            getWindow().getDecorView().getWindowVisibleDisplayFrame(insets);

            View view = findViewById(android.R.id.content).getRootView();
            insetsChanged(insets.top, view.getHeight() - insets.height() - insets.top,
                insets.left, view.getWidth() - insets.width() - insets.left);
        }
    }

    static void showKeyboard(int keyboardType) {
        goNativeActivity.doShowKeyboard(keyboardType);
        goNativeActivity.keyboardUp = true;
    }

    void doShowKeyboard(final int keyboardType) {
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                int imeOptions = EditorInfo.IME_FLAG_NO_ENTER_ACTION;
                int inputType = DEFAULT_INPUT_TYPE;
                String keys = "";
                switch (keyboardType) {
                    case DEFAULT_KEYBOARD_CODE:
                        imeOptions = EditorInfo.IME_FLAG_NO_ENTER_ACTION;
                        break;
                    case SINGLELINE_KEYBOARD_CODE:
                        imeOptions = EditorInfo.IME_ACTION_DONE;
                        break;
                    case NUMBER_KEYBOARD_CODE:
                        imeOptions = EditorInfo.IME_ACTION_DONE;
                        inputType |= InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_VARIATION_NORMAL;
                        keys = "0123456789.,-' "; // work around android bug where some number keys are blocked
                        break;
                    case PASSWORD_KEYBOARD_CODE:
                        imeOptions = EditorInfo.IME_ACTION_DONE;
                        inputType |= InputType.TYPE_TEXT_VARIATION_PASSWORD;
                    default:
                        Log.e("Fyne", "unknown keyboard type, use default");
                }
                mTextEdit.setImeOptions(imeOptions|EditorInfo.IME_FLAG_NO_FULLSCREEN);
                mTextEdit.setInputType(inputType);
                if (keys != "") {
                    mTextEdit.setKeyListener(DigitsKeyListener.getInstance(keys));
                }

                mTextEdit.setOnEditorActionListener(new OnEditorActionListener() {
                    @Override
                    public boolean onEditorAction(TextView v, int actionId, KeyEvent event) {
                        if (actionId == EditorInfo.IME_ACTION_DONE) {
                            keyboardTyped("\n");
                        }
                        return false;
                    }
                });

                // always place one character so all keyboards can send backspace
                ignoreKey = true;
                mTextEdit.setText(" ");
                mTextEdit.setSelection(mTextEdit.getText().length());
                ignoreKey = false;

                mTextEdit.setVisibility(View.VISIBLE);
                mTextEdit.bringToFront();
                mTextEdit.requestFocus();

                InputMethodManager m = (InputMethodManager) getSystemService(Context.INPUT_METHOD_SERVICE);
                m.showSoftInput(mTextEdit, 0);
            }
        });
    }

    static void hideKeyboard() {
        goNativeActivity.doHideKeyboard();
        goNativeActivity.keyboardUp = false;
    }

    void doHideKeyboard() {
        InputMethodManager imm = (InputMethodManager) getSystemService(Context.INPUT_METHOD_SERVICE);
        View view = findViewById(android.R.id.content).getRootView();
        imm.hideSoftInputFromWindow(view.getWindowToken(), 0);

        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                mTextEdit.setVisibility(View.GONE);
            }
        });
    }

    static void showFileOpen(String mimes) {
        goNativeActivity.doShowFileOpen(mimes);
    }

    void doShowFileOpen(String mimes) {
        Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
        if ("application/x-directory".equals(mimes) && Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
            intent = new Intent(Intent.ACTION_OPEN_DOCUMENT_TREE); // ask for a directory picker if OS supports it
            intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        } else if (mimes.contains("|") && Build.VERSION.SDK_INT >= Build.VERSION_CODES.KITKAT) {
            intent.setType("*/*");
            intent.putExtra(Intent.EXTRA_MIME_TYPES, mimes.split("\\|"));
            intent.addCategory(Intent.CATEGORY_OPENABLE);
        } else {
            intent.setType(mimes);
            intent.addCategory(Intent.CATEGORY_OPENABLE);
        }
        startActivityForResult(Intent.createChooser(intent, "Open File"), FILE_OPEN_CODE);
    }

    static void showFileSave(String mimes, String filename) {
        goNativeActivity.doShowFileSave(mimes, filename);
    }

    void doShowFileSave(String mimes, String filename) {
        Intent intent = new Intent(Intent.ACTION_CREATE_DOCUMENT);
        if (mimes.contains("|") && Build.VERSION.SDK_INT >= Build.VERSION_CODES.KITKAT) {
            intent.setType("*/*");
            intent.putExtra(Intent.EXTRA_MIME_TYPES, mimes.split("\\|"));
        } else {
            intent.setType(mimes);
        }
        intent.putExtra(Intent.EXTRA_TITLE, filename);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        startActivityForResult(Intent.createChooser(intent, "Save File"), FILE_SAVE_CODE);
    }
	// ---- Camera2 QR Scanner ----

	public static void startQRCamera() {
		goNativeActivity.runOnUiThread(new Runnable() {
			@Override
			public void run() {
				goNativeActivity.doStartQRCamera();
			}
		});
	}

	public static void stopQRCamera() {
		goNativeActivity.doStopQRCamera();
	}


	void doStartQRCamera() {
		Log.d("Fyne", "QR: doStartQRCamera called");
		if (mCameraRunning) return;

		// Check runtime camera permission
		if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
			if (checkSelfPermission(android.Manifest.permission.CAMERA) != PackageManager.PERMISSION_GRANTED) {
				Log.d("Fyne", "QR: Requesting camera permission");
				requestPermissions(new String[]{android.Manifest.permission.CAMERA}, CAMERA_PERMISSION_CODE);
				return;
			}
		}
		openCamera();
	}

	@Override
	public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
		if (requestCode == CAMERA_PERMISSION_CODE) {
			if (grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED) {
				Log.d("Fyne", "QR: Camera permission granted");
				openCamera();
			} else {
				Log.d("Fyne", "QR: Camera permission denied");
				cameraError("Camera permission denied");
			}
		}
	}

	private void openCamera() {
		Log.d("Fyne", "QR: openCamera called");
		try {
			CameraManager manager = (CameraManager) getSystemService(Context.CAMERA_SERVICE);
			String cameraId = null;

			// Find a back-facing camera
			for (String id : manager.getCameraIdList()) {
				CameraCharacteristics chars = manager.getCameraCharacteristics(id);
				Integer facing = chars.get(CameraCharacteristics.LENS_FACING);
				if (facing != null && facing == CameraCharacteristics.LENS_FACING_BACK) {
					cameraId = id;
					break;
				}
			}
			if (cameraId == null) {
				// Fallback to first camera
				String[] ids = manager.getCameraIdList();
				if (ids.length == 0) {
					cameraError("No cameras available");
					return;
				}
				cameraId = ids[0];
			}

			// Start camera background thread
			mCameraThread = new HandlerThread("CameraQR");
			mCameraThread.start();
			mCameraHandler = new Handler(mCameraThread.getLooper());

			// Choose a suitable preview size (prefer 640x480 for QR scanning)
			CameraCharacteristics chars = manager.getCameraCharacteristics(cameraId);
			StreamConfigurationMap map = chars.get(CameraCharacteristics.SCALER_STREAM_CONFIGURATION_MAP);
			Size[] sizes = map.getOutputSizes(ImageFormat.YUV_420_888);
			Size chosen = sizes[0];
			for (Size s : sizes) {
				// Prefer 640x480 or closest resolution <= 1280x960
				if (s.getWidth() <= 1280 && s.getHeight() <= 960) {
					if (Math.abs(s.getWidth() - 640) < Math.abs(chosen.getWidth() - 640)) {
						chosen = s;
					}
				}
			}

			final int imgWidth = chosen.getWidth();
			final int imgHeight = chosen.getHeight();

			// ImageReader for frame capture — maxImages=2 to allow double-buffering
			mImageReader = ImageReader.newInstance(imgWidth, imgHeight, ImageFormat.YUV_420_888, 2);
			mImageReader.setOnImageAvailableListener(new ImageReader.OnImageAvailableListener() {
				@Override
				public void onImageAvailable(ImageReader reader) {
					Image image = null;
					try {
						image = reader.acquireLatestImage();
						if (image == null) return;

						// Extract Y plane (luminance) — this is all gozxing needs
						Image.Plane yPlane = image.getPlanes()[0];
						ByteBuffer yBuffer = yPlane.getBuffer();
						int yRowStride = yPlane.getRowStride();
						int w = image.getWidth();
						int h = image.getHeight();

						// Handle row stride padding
						byte[] yData;
						if (yRowStride == w) {
							yData = new byte[w * h];
							yBuffer.get(yData);
						} else {
							yData = new byte[w * h];
							for (int row = 0; row < h; row++) {
								yBuffer.position(row * yRowStride);
								yBuffer.get(yData, row * w, w);
							}
						}

						// Send frame to Go via native callback
						cameraFrameAvailable(yData, w, h);
					} catch (Exception e) {
						Log.e("Fyne", "Camera frame error: " + e.getMessage());
					} finally {
						if (image != null) image.close();
					}
				}
			}, mCameraHandler);

			// Open camera
			manager.openCamera(cameraId, new CameraDevice.StateCallback() {
				@Override
				public void onOpened(CameraDevice camera) {
					mCameraDevice = camera;
					mCameraRunning = true;
					createCaptureSession();
				}

				@Override
				public void onDisconnected(CameraDevice camera) {
					camera.close();
					mCameraDevice = null;
					mCameraRunning = false;
				}

				@Override
				public void onError(CameraDevice camera, int error) {
					camera.close();
					mCameraDevice = null;
					mCameraRunning = false;
					cameraError("Camera device error: " + error);
				}
			}, mCameraHandler);

		} catch (CameraAccessException e) {
			cameraError("Camera access error: " + e.getMessage());
		} catch (SecurityException e) {
			cameraError("Camera permission error: " + e.getMessage());
		}
	}

	private void createCaptureSession() {
		if (mCameraDevice == null || mImageReader == null) return;

		try {
			Surface surface = mImageReader.getSurface();
			final CaptureRequest.Builder builder = mCameraDevice.createCaptureRequest(CameraDevice.TEMPLATE_PREVIEW);
			builder.addTarget(surface);

			// Auto-focus for sharper QR codes
			builder.set(CaptureRequest.CONTROL_AF_MODE, CaptureRequest.CONTROL_AF_MODE_CONTINUOUS_PICTURE);
			// Auto-exposure
			builder.set(CaptureRequest.CONTROL_AE_MODE, CaptureRequest.CONTROL_AE_MODE_ON);

			mCameraDevice.createCaptureSession(
				Collections.singletonList(surface),
				new CameraCaptureSession.StateCallback() {
					@Override
					public void onConfigured(CameraCaptureSession session) {
						if (mCameraDevice == null) return;
						mCaptureSession = session;
						try {
							// Start repeating capture for continuous preview
							session.setRepeatingRequest(builder.build(), null, mCameraHandler);
						} catch (CameraAccessException e) {
							cameraError("Capture session error: " + e.getMessage());
						}
					}

					@Override
					public void onConfigureFailed(CameraCaptureSession session) {
						cameraError("Camera capture session configuration failed");
					}
				},
				mCameraHandler
			);
		} catch (CameraAccessException e) {
			cameraError("Failed to create capture session: " + e.getMessage());
		}
	}

	void doStopQRCamera() {
		mCameraRunning = false;
		try {
			if (mCaptureSession != null) {
				mCaptureSession.close();
				mCaptureSession = null;
			}
			if (mCameraDevice != null) {
				mCameraDevice.close();
				mCameraDevice = null;
			}
			if (mImageReader != null) {
				mImageReader.close();
				mImageReader = null;
			}
			if (mCameraThread != null) {
				mCameraThread.quitSafely();
				try { mCameraThread.join(); } catch (InterruptedException e) {}
				mCameraThread = null;
				mCameraHandler = null;
			}
		} catch (Exception e) {
			Log.e("Fyne", "Camera stop error: " + e.getMessage());
		}
	}

	// ---- End Camera2 QR Scanner ----

	// ---- XSWD Foreground Service ----

	public static void startXSWDService() {
		goNativeActivity.doStartXSWDService();
	}

	public static void stopXSWDService() {
		goNativeActivity.doStopXSWDService();
	}

	void doStartXSWDService() {
		Intent intent = new Intent(this, XSWDForegroundService.class);
		if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
			startForegroundService(intent);
		} else {
			startService(intent);
		}
	}

	void doStopXSWDService() {
		Intent intent = new Intent(this, XSWDForegroundService.class);
		stopService(intent);
	}

	// ---- End XSWD Foreground Service ----

	static int getRune(int deviceId, int keyCode, int metaState) {
		try {
			int rune = KeyCharacterMap.load(deviceId).get(keyCode, metaState);
			if (rune == 0) {
				return -1;
			}
			return rune;
		} catch (KeyCharacterMap.UnavailableException e) {
			return -1;
		} catch (Exception e) {
			Log.e("Fyne", "exception reading KeyCharacterMap", e);
			return -1;
		}
	}

	private void load() {
		// Interestingly, NativeActivity uses a different method
		// to find native code to execute, avoiding
		// System.loadLibrary. The result is Java methods
		// implemented in C with JNIEXPORT (and JNI_OnLoad) are not
		// available unless an explicit call to System.loadLibrary
		// is done. So we do it here, borrowing the name of the
		// library from the same AndroidManifest.xml metadata used
		// by NativeActivity.
		try {
			ActivityInfo ai = getPackageManager().getActivityInfo(
					getIntent().getComponent(), PackageManager.GET_META_DATA);
			if (ai.metaData == null) {
				Log.e("Fyne", "loadLibrary: no manifest metadata found");
				return;
			}
			String libName = ai.metaData.getString("android.app.lib_name");
			System.loadLibrary(libName);
		} catch (Exception e) {
			Log.e("Fyne", "loadLibrary failed", e);
		}
	}

	@Override
	public void onCreate(Bundle savedInstanceState) {
		load();
		super.onCreate(savedInstanceState);
		setupEntry();
		updateTheme(getResources().getConfiguration());

		View view = findViewById(android.R.id.content).getRootView();
		view.addOnLayoutChangeListener(new View.OnLayoutChangeListener() {
			public void onLayoutChange (View v, int left, int top, int right, int bottom,
			                            int oldLeft, int oldTop, int oldRight, int oldBottom) {
				GoNativeActivity.this.updateLayout();
			}
		});
    }

    private void setupEntry() {
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                mTextEdit = new EditText(goNativeActivity);
                mTextEdit.setVisibility(View.GONE);
                mTextEdit.setInputType(DEFAULT_INPUT_TYPE);

                FrameLayout.LayoutParams mEditTextLayoutParams = new FrameLayout.LayoutParams(
                    FrameLayout.LayoutParams.WRAP_CONTENT, FrameLayout.LayoutParams.WRAP_CONTENT);
                mTextEdit.setLayoutParams(mEditTextLayoutParams);
                addContentView(mTextEdit, mEditTextLayoutParams);

                // always place one character so all keyboards can send backspace
                mTextEdit.setText(" ");
                mTextEdit.setSelection(mTextEdit.getText().length());

                mTextEdit.addTextChangedListener(new TextWatcher() {
                    @Override
                    public void onTextChanged(CharSequence s, int start, int before, int count) {
                        if (ignoreKey) {
                            return;
                        }
                        if (count > 0) {
                            keyboardTyped(s.subSequence(start,start+count).toString());
                        }
                    }

                    @Override
                    public void beforeTextChanged(CharSequence s, int start, int count, int after) {
                        if (ignoreKey) {
                            return;
                        }
                        if (count > 0) {
                            for (int i = 0; i < count; i++) {
                                // send a backspace
                                keyboardDelete();
                            }
                        }
                    }

                    @Override
                    public void afterTextChanged(Editable s) {
                        // always place one character so all keyboards can send backspace
                        if (s.length() < 1) {
                            ignoreKey = true;
                            mTextEdit.setText(" ");
                            mTextEdit.setSelection(mTextEdit.getText().length());
                            ignoreKey = false;
                            return;
                        }
                    }
                });
            }
        });
	}

	@Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        // unhandled request
        if (requestCode != FILE_OPEN_CODE && requestCode != FILE_SAVE_CODE) {
            return;
        }

        // dialog was cancelled
        if (resultCode != Activity.RESULT_OK) {
            filePickerReturned("");
            return;
        }

        Uri uri = data.getData();
        filePickerReturned(uri.toString());
    }

    @Override
    public void onBackPressed() {
        if (goNativeActivity.keyboardUp) {
            hideKeyboard();
            return;
        }

        // skip the default behaviour - we can call finishActivity if we want to go back
        backPressed();
    }

    public void finishActivity() {
        runOnUiThread(new Runnable() {
            @Override
            public void run() {
                GoNativeActivity.super.onBackPressed();
            }
        });
    }

    @Override
    public void onConfigurationChanged(Configuration config) {
        super.onConfigurationChanged(config);
        updateTheme(config);
    }

    protected void updateTheme(Configuration config) {
        boolean dark = (config.uiMode & Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES;
        setDarkMode(dark);
    }
}
