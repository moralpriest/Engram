//go:build android

package main

/*
#include <jni.h>

void startXSWDForegroundService(JNIEnv* env);
void stopXSWDForegroundService(JNIEnv* env);
*/
import "C"

import (
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

func startXSWDForegroundAndroid() {
	driver.RunNative(func(ctx any) error {
		ac, ok := ctx.(*driver.AndroidContext)
		if !ok {
			return nil
		}
		C.startXSWDForegroundService((*C.JNIEnv)(unsafe.Pointer(ac.Env)))
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
