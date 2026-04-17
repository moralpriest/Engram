//go:build android

package camera

/*
#cgo LDFLAGS: -landroid -llog

#include <jni.h>
#include <stdlib.h>
#include <android/log.h>

// Version marker V5 - FINAL CACHE BREAK
#define JNI_VERSION_TAG "JNI-V5-FINAL"
#define LOG_TAG "Fyne-QR-JNI"
#define LOGI(...) __android_log_print(ANDROID_LOG_INFO, LOG_TAG, __VA_ARGS__)
#define LOGE(...) __android_log_print(ANDROID_LOG_ERROR, LOG_TAG, __VA_ARGS__)

static int native_req_qr_v5(uintptr_t jvm_ptr, uintptr_t activity_ptr) {
    JavaVM* jvm = (JavaVM*)jvm_ptr;
    jobject activity = (jobject)activity_ptr;
    JNIEnv* env = NULL;

    LOGI("native_req_qr_v5 [%s] - Starting hard trigger", JNI_VERSION_TAG);

    jint res = (*jvm)->GetEnv(jvm, (void**)&env, JNI_VERSION_1_6);
    if (res != JNI_OK) {
        LOGI("Attaching current thread to JVM");
        if ((*jvm)->AttachCurrentThread(jvm, (void**)&env, NULL) != 0) {
            LOGE("Failed to attach thread to JVM");
            return -1;
        }
    }

    if (env == NULL) {
        LOGE("Failed to get JNIEnv");
        return -1;
    }

    jclass cls = (*env)->GetObjectClass(env, activity);
    if (cls == NULL) {
        LOGE("Could not get class from activity object");
        return -1;
    }

    // Using V5 name to definitively break shadow caches
    jmethodID mid = (*env)->GetStaticMethodID(env, cls, "triggerScanner_V5", "()V");
    if (mid == NULL) {
        LOGE("Static Method triggerScanner_V5()V not found in Activity - Check APK manifests");
        return -2;
    }

    LOGI("Calling static triggerScanner_V5 via JNI");
    (*env)->CallStaticVoidMethod(env, cls, mid);

    if ((*env)->ExceptionCheck(env)) {
        LOGE("Exception occurred during static triggerScanner_V5 call");
        (*env)->ExceptionClear(env);
        return -3;
    }

    LOGI("triggerScanner_V5 triggered successfully");
    return 0;
}

static const char* get_jni_string(JNIEnv* env, jstring str) {
    if (str == NULL) return NULL;
    return (*env)->GetStringUTFChars(env, str, NULL);
}

static void release_jni_string(JNIEnv* env, jstring str, const char* ptr) {
    if (str != NULL && ptr != NULL) {
        (*env)->ReleaseStringUTFChars(env, str, ptr);
    }
}
*/
import "C"

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"log"
	"sync"
)

var (
	activeScanner *AndroidScanner
	scannerMu     sync.Mutex
)

type AndroidScanner struct {
	window fyne.Window
	onScan func(string)
}

func NewScanner(win fyne.Window, onScan func(string)) *AndroidScanner {
	return &AndroidScanner{
		window: win,
		onScan: onScan,
	}
}

func (s *AndroidScanner) Start() error {
	scannerMu.Lock()
	activeScanner = s
	scannerMu.Unlock()

	return driver.RunNative(func(ctx any) error {
		ac := ctx.(*driver.AndroidContext)
		ret := int(C.native_req_qr_v5(C.uintptr_t(ac.VM), C.uintptr_t(ac.Ctx)))
		
		switch ret {
		case -1:
			log.Println("QR: Failed to initialize JNI bridge context")
		case -2:
			log.Println("QR: triggerScanner_V5 method not found in GoNativeActivity")
		case -3:
			log.Println("QR: Exception occurred during Java call")
		case 0:
			log.Println("QR: Scanner trigger sent to Java successfully")
		}
		return nil
	})
}

func (s *AndroidScanner) Stop() {
	scannerMu.Lock()
	if activeScanner == s {
		activeScanner = nil
	}
	scannerMu.Unlock()
}

//export Java_org_golang_app_GoNativeActivity_sendQRResultNative
func Java_org_golang_app_GoNativeActivity_sendQRResultNative(env *C.JNIEnv, obj C.jobject, result C.jstring) {
	cStr := C.get_jni_string(env, result)
	if cStr == nil {
		return
	}
	defer C.release_jni_string(env, result, cStr)

	resStr := C.GoString(cStr)
	log.Printf("QR: Native callback received result: %s", resStr)

	scannerMu.Lock()
	if activeScanner != nil && activeScanner.onScan != nil {
		cb := activeScanner.onScan
		go cb(resStr)
	}
	scannerMu.Unlock()
}
