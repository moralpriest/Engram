//go:build !amd64.v3

package astrobwt

import "unsafe"

func buildEqualColumns(first *byte, groupCount uint32, out *[4]uint64) {
	for i := range out {
		out[i] = ^uint64(0)
	}
	base := unsafe.Pointer(first)
	for group := uint32(1); group < groupCount; group++ {
		current := unsafe.Add(base, group<<8)
		for word := uint32(0); word < 32; word++ {
			offset := word << 3
			diff := *(*uint64)(unsafe.Add(base, offset)) ^
				*(*uint64)(unsafe.Add(current, offset))
			if diff == 0 {
				continue
			}
			for b := uint32(0); b < 8; b++ {
				if byte(diff>>(b<<3)) != 0 {
					rel := offset + b
					out[rel>>6] &^= uint64(1) << (rel & 63)
				}
			}
		}
	}
}
