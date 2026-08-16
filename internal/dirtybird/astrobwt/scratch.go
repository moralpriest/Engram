package astrobwt

// Scratch layout derived from derohe astrobwt/astrobwtv3/sa_fast.go
// (DERO Foundation, see LICENSE-DERO.txt). The dead sort_indices path and its
// 786KB of buffers are dropped; buffers are sized to the true maximum stream
// length instead of derohe's MAX_LENGTH.

import "unsafe"

// The wolf loop runs at most 277 iterations, each appending 256 bytes, so the
// data stream never exceeds 277*256 = 70912 bytes and
// data_len = (tries-4)*256 + 0x3ff caps at 70911.
const MAX_LENGTH uint32 = 277 * 256

type ScratchData struct {
	data     [MAX_LENGTH + 64]uint8
	sa       [MAX_LENGTH + 8]int32
	sa_bytes *[(MAX_LENGTH) * 4]uint8

	// v114 descriptor-SA state: template markers recorded by the wolf loop
	// (free bookkeeping) and the lazily-allocated stage-4/5 buffers.
	markers    [280]uint16
	nTemplates uint32
	flags      [280]byte
	useV114    bool
	v114       *v114Scratch
	saisTmp    [MAX_LENGTH / 2]int32
}

func NewScratchData() *ScratchData {
	d := &ScratchData{}
	d.sa_bytes = ((*[(MAX_LENGTH) * 4]byte)(unsafe.Pointer(&d.sa[0])))
	return d
}

func text_32_0alloc(text []byte, sa, tmp []int32) {
	if int(int32(len(text))) != len(text) || len(text) != len(sa) {
		panic("suffixarray: misuse of text_32")
	}
	for i := range sa {
		sa[i] = 0
	}
	sais_8_32(text, 256, sa, tmp)
}
