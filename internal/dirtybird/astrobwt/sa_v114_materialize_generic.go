//go:build !amd64.v3

package astrobwt

import "unsafe"

const uniqueRunBatchAvailable = false

func materializeOrigins(dst, src *uint32, count, rel uint32) {
	d := unsafe.Slice(dst, count)
	s := unsafe.Slice(src, count)
	for i := range d {
		d[i] = s[i] + rel
	}
}

func writeUniqueRunBatch(_ *uint32, _ *stage5Run, _ *uint32, _, groupStart, outPos, _, _ int) (int, int) {
	return groupStart, outPos
}
