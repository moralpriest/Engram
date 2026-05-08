// Copyright 2023-2026 DERO Foundation. All rights reserved.
// Camera2 frame buffer for QR scanner (thread-safe C shared buffer)

//go:build android

#include <jni.h>
#include <pthread.h>
#include <stdlib.h>
#include <string.h>
#include <android/log.h>

#define LOG_TAG "EngramScanner"
#define LOGD(...) __android_log_print(ANDROID_LOG_DEBUG, LOG_TAG, __VA_ARGS__)

static pthread_mutex_t g_frame_mutex = PTHREAD_MUTEX_INITIALIZER;
static unsigned char* g_frame_buf = NULL;
static int g_frame_buf_cap = 0;
static int g_frame_width = 0;
static int g_frame_height = 0;
static int g_frame_new = 0;

static pthread_mutex_t g_error_mutex = PTHREAD_MUTEX_INITIALIZER;
static char g_error_msg[256] = {0};
static int g_error_new = 0;

// Explicitly export these for the JNI linker
JNIEXPORT void JNICALL Java_org_golang_app_GoNativeActivity_cameraFrameAvailable(
        JNIEnv *env, jclass clazz, jbyteArray data, jint width, jint height) {
    
    // LOGD("JNI: Frame %dx%d", width, height); // Commented out to avoid log spam, enable if needed
    
    jint length = (*env)->GetArrayLength(env, data);
    int needed = (int)(width) * (int)(height);
    if (needed <= 0 || length < needed) return;

    pthread_mutex_lock(&g_frame_mutex);

    if (g_frame_buf == NULL || g_frame_buf_cap < needed) {
        if (g_frame_buf) free(g_frame_buf);
        g_frame_buf = (unsigned char*)malloc(needed);
        g_frame_buf_cap = needed;
    }
    if (g_frame_buf != NULL) {
        (*env)->GetByteArrayRegion(env, data, 0, needed, (jbyte*)g_frame_buf);
        g_frame_width = (int)width;
        g_frame_height = (int)height;
        g_frame_new = 1;
    }

    pthread_mutex_unlock(&g_frame_mutex);
}

JNIEXPORT void JNICALL Java_org_golang_app_GoNativeActivity_cameraError(
        JNIEnv *env, jclass clazz, jstring str) {
    if (!str) return;
    const char* cstr = (*env)->GetStringUTFChars(env, str, NULL);
    if (!cstr) return;

    LOGD("JNI: cameraError: %s", cstr);

    pthread_mutex_lock(&g_error_mutex);
    strncpy(g_error_msg, cstr, sizeof(g_error_msg) - 1);
    g_error_msg[sizeof(g_error_msg) - 1] = '\0';
    g_error_new = 1;
    pthread_mutex_unlock(&g_error_mutex);

    (*env)->ReleaseStringUTFChars(env, str, cstr);
}

// Internal polling functions for Go
int pollCameraFrame(unsigned char* outBuf, int outBufSize, int* outWidth, int* outHeight) {
    int result = 0;
    pthread_mutex_lock(&g_frame_mutex);
    if (g_frame_new && g_frame_buf != NULL) {
        int sz = g_frame_width * g_frame_height;
        if (sz > 0 && sz <= outBufSize) {
            memcpy(outBuf, g_frame_buf, sz);
            *outWidth = g_frame_width;
            *outHeight = g_frame_height;
            result = 1;
        }
        g_frame_new = 0;
    }
    pthread_mutex_unlock(&g_frame_mutex);
    return result;
}

int pollCameraError(char* outBuf, int outBufSize) {
    int result = 0;
    pthread_mutex_lock(&g_error_mutex);
    if (g_error_new) {
        strncpy(outBuf, g_error_msg, outBufSize - 1);
        outBuf[outBufSize - 1] = '\0';
        g_error_new = 0;
        result = 1;
    }
    pthread_mutex_unlock(&g_error_mutex);
    return result;
}

void cleanupCameraBuffers() {
    pthread_mutex_lock(&g_frame_mutex);
    if (g_frame_buf) free(g_frame_buf);
    g_frame_buf = NULL;
    g_frame_buf_cap = 0;
    g_frame_new = 0;
    pthread_mutex_unlock(&g_frame_mutex);

    pthread_mutex_lock(&g_error_mutex);
    g_error_msg[0] = '\0';
    g_error_new = 0;
    pthread_mutex_unlock(&g_error_mutex);
}
