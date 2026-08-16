//go:build windows

package miner

import (
	"encoding/binary"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                              = windows.NewLazySystemDLL("kernel32.dll")
	procSetThreadAffinityMask             = kernel32.NewProc("SetThreadAffinityMask")
	procGetLogicalProcessorInformationEx  = kernel32.NewProc("GetLogicalProcessorInformationEx")
)

// PinOrder returns the CPU order workers pin to: primary thread of each core,
// most-performant efficiency class first (P-cores before E-cores on hybrid
// parts), then the remaining SMT siblings. Falls back to the official miner's
// avoidHT interleave if topology can't be read.
func PinOrder() []int {
	if order := pCoreFirstOrder(); order != nil {
		return order
	}
	return interleaveOrder()
}

func interleaveOrder() []int {
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

const relationProcessorCore = 0

// pCoreFirstOrder parses GetLogicalProcessorInformationEx(RelationProcessorCore)
// records by hand (x/sys does not wrap them). Record layout, winnt.h:
//
//	SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX: Relationship u32, Size u32, union
//	PROCESSOR_RELATIONSHIP @8: Flags u8, EfficiencyClass u8, Reserved[20],
//	  GroupCount u16 @30, GroupMask []GROUP_AFFINITY @32
//	GROUP_AFFINITY (16 bytes): Mask u64, Group u16, Reserved[3]u16
//
// Group 0 only; returns nil on any parse trouble.
func pCoreFirstOrder() []int {
	var size uint32
	procGetLogicalProcessorInformationEx.Call(relationProcessorCore, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 || size > 1<<20 {
		return nil
	}
	buf := make([]byte, size)
	r1, _, _ := procGetLogicalProcessorInformationEx.Call(relationProcessorCore,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r1 == 0 {
		return nil
	}

	type core struct {
		class byte
		lps   []int
	}
	var cores []core
	le := binary.LittleEndian
	for off := uint32(0); off+8 <= size; {
		relationship := le.Uint32(buf[off:])
		recSize := le.Uint32(buf[off+4:])
		if recSize == 0 || off+recSize > size {
			return nil
		}
		if relationship == relationProcessorCore && recSize >= 32+16 {
			class := buf[off+9]
			groupCount := le.Uint16(buf[off+30:])
			var lps []int
			for g := uint16(0); g < groupCount; g++ {
				gaOff := off + 32 + uint32(g)*16
				if gaOff+16 > off+recSize {
					break
				}
				mask := le.Uint64(buf[gaOff:])
				group := le.Uint16(buf[gaOff+8:])
				if group != 0 { // single-group machines only
					continue
				}
				for bit := 0; bit < 64; bit++ {
					if mask&(1<<uint(bit)) != 0 {
						lps = append(lps, bit)
					}
				}
			}
			if len(lps) > 0 {
				cores = append(cores, core{class: class, lps: lps})
			}
		}
		off += recSize
	}
	if len(cores) == 0 {
		return nil
	}

	// stable sort by efficiency class, highest (most performant) first
	for i := 1; i < len(cores); i++ {
		for j := i; j > 0 && cores[j].class > cores[j-1].class; j-- {
			cores[j], cores[j-1] = cores[j-1], cores[j]
		}
	}
	var order []int
	for _, c := range cores { // primary thread of each core
		order = append(order, c.lps[0])
	}
	for _, c := range cores { // then SMT siblings
		for _, lp := range c.lps[1:] {
			order = append(order, lp)
		}
	}
	return order
}

func pinCurrentThread(tid int, order []int) {
	if tid >= len(order) {
		return
	}
	cpu := order[tid]
	if cpu >= 64 {
		return
	}
	procSetThreadAffinityMask.Call(uintptr(windows.CurrentThread()), uintptr(1)<<uint(cpu))
}

// PinThreadForBench pins the calling OS thread (which must be locked) for
// bench-mode workers, same as mining workers.
func PinThreadForBench(tid int, order []int) { pinCurrentThread(tid, order) }

// SetHighPriority raises the process to HIGH_PRIORITY_CLASS.
func SetHighPriority() error {
	return windows.SetPriorityClass(windows.CurrentProcess(), windows.HIGH_PRIORITY_CLASS)
}
