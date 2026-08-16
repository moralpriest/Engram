//go:build !windows

package miner

import "runtime"

// PinOrder returns the avoidHT interleave; per-thread pinning is a no-op off
// Windows in v1 (matching the family's Windows-first focus).
func PinOrder() []int {
	n := runtime.NumCPU()
	order := make([]int, 0, n)
	for i := 0; i < n; i += 2 {
		order = append(order, i)
	}
	for i := 1; i < n; i += 2 {
		order = append(order, i)
	}
	return order
}

func pinCurrentThread(tid int, order []int) {}

// PinThreadForBench is a no-op off Windows, like pinCurrentThread.
func PinThreadForBench(tid int, order []int) {}

func SetHighPriority() error { return nil }
