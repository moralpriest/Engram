//go:build linux

package main

import (
	"runtime"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

var processor int32

func threadaffinity() {
	var cpuset unix.CPUSet
	lock_on_cpu := atomic.AddInt32(&processor, 1)
	if lock_on_cpu >= int32(runtime.GOMAXPROCS(0)) {
		return
	}
	cpuset.Zero()
	cpuset.Set(int(avoidHT(int(lock_on_cpu))))
	unix.SchedSetaffinity(0, &cpuset)
}

func avoidHT(i int) int {
	count := runtime.GOMAXPROCS(0)
	if i < count/2 {
		return i * 2
	}
	return (i-count/2)*2 + 1
}
