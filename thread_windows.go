//go:build windows

package main

import (
	"math/bits"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	libkernel32          uintptr
	setThreadAffinityPtr uintptr
)

func doLoadLibrary(name string) uintptr {
	lib, _ := syscall.LoadLibrary(name)
	return uintptr(lib)
}

func doGetProcAddress(lib uintptr, name string) uintptr {
	addr, _ := syscall.GetProcAddress(syscall.Handle(lib), name)
	return uintptr(addr)
}

func syscall3(trap, nargs, a1, a2, a3 uintptr) uintptr {
	ret, _, _ := syscall.Syscall(trap, nargs, a1, a2, a3)
	return ret
}

func init() {
	libkernel32 = doLoadLibrary("kernel32.dll")
	setThreadAffinityPtr = doGetProcAddress(libkernel32, "SetThreadAffinityMask")
}

var processor int32

func setThreadAffinityMask(hThread syscall.Handle, dwThreadAffinityMask uint) *uint32 {
	ret1 := syscall3(setThreadAffinityPtr, 2, uintptr(hThread), uintptr(dwThreadAffinityMask), 0)
	return (*uint32)(unsafe.Pointer(ret1))
}

func currentThread() syscall.Handle {
	return syscall.Handle(^uintptr(2 - 1))
}

func threadaffinity() {
	lock_on_cpu := atomic.AddInt32(&processor, 1)
	if lock_on_cpu >= int32(runtime.GOMAXPROCS(0)) {
		return
	}
	if lock_on_cpu >= bits.UintSize {
		return
	}
	var cpuset uint
	cpuset = 1 << uint(avoidHT(int(lock_on_cpu)))
	setThreadAffinityMask(currentThread(), cpuset)
}

func avoidHT(i int) int {
	count := runtime.GOMAXPROCS(0)
	if i < count/2 {
		return i * 2
	}
	return (i-count/2)*2 + 1
}
