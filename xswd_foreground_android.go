//go:build android

package main

/*
#include <stdlib.h>
#include <jni.h>

void startXSWDForegroundService(JNIEnv* env, const char* title, const char* text, const char* channelName, const char* channelDesc);
void stopXSWDForegroundService(JNIEnv* env);
*/
import "C"

import (
	"unsafe"

	"fyne.io/fyne/v2/driver"

	"github.com/DEROFDN/engram/i18n"
)

func startXSWDForegroundAndroid() {
	title := C.CString("Engram")
	text := C.CString(i18n.T("xswd.foreground_notification_text"))
	channelName := C.CString(i18n.T("xswd.foreground_channel_name"))
	channelDesc := C.CString(i18n.T("xswd.foreground_channel_description"))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(text))
	defer C.free(unsafe.Pointer(channelName))
	defer C.free(unsafe.Pointer(channelDesc))

	driver.RunNative(func(ctx any) error {
		ac, ok := ctx.(*driver.AndroidContext)
		if !ok {
			return nil
		}
		C.startXSWDForegroundService((*C.JNIEnv)(unsafe.Pointer(ac.Env)), title, text, channelName, channelDesc)
		return nil
	})
}

func stopXSWDForegroundAndroid() {
	driver.RunNative(func(ctx any) error {
		ac, ok := ctx.(*driver.AndroidContext)
		if !ok {
			return nil
		}
		C.stopXSWDForegroundService((*C.JNIEnv)(unsafe.Pointer(ac.Env)))
		return nil
	})
}

func startAppForegroundAndroid() {
	title := C.CString("Engram")
	text := C.CString(i18n.T("app.foreground_notification_text"))
	channelName := C.CString(i18n.T("app.foreground_channel_name"))
	channelDesc := C.CString(i18n.T("app.foreground_channel_description"))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(text))
	defer C.free(unsafe.Pointer(channelName))
	defer C.free(unsafe.Pointer(channelDesc))

	driver.RunNative(func(ctx any) error {
		ac, ok := ctx.(*driver.AndroidContext)
		if !ok {
			return nil
		}
		C.startXSWDForegroundService((*C.JNIEnv)(unsafe.Pointer(ac.Env)), title, text, channelName, channelDesc)
		return nil
	})
}
