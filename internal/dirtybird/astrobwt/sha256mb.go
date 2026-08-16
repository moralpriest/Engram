//go:build amd64

package astrobwt

// 2-way multi-buffer SHA-256 over SHA-NI (sha256mb_amd64.s). Single-stream
// SHA-NI is latency-bound (each sha256rnds2 depends on the previous), so
// hashing two independent messages with interleaved instruction streams lets
// the out-of-order engine overlap the chains — ~1.3x throughput on Raptor
// Cove (capped by its single shared SHA port). Same design as the zig
// miner's sha256_mb.zig; digests are byte-identical to crypto/sha256.

import (
	"encoding/binary"

	"github.com/klauspost/cpuid/v2"
)

var useSHANI = cpuid.CPU.Supports(cpuid.SHA, cpuid.SSSE3, cpuid.SSE4)

var sha256IV = [8]uint32{
	0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
	0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
}

// Implemented in sha256mb_amd64.s (pure legacy-SSE SHA-NI).
//
//go:noescape
func sha256BlocksNI(state *[8]uint32, p *byte, nblocks int)

//go:noescape
func sha256Blocks2NI(state0 *[8]uint32, p0 *byte, state1 *[8]uint32, p1 *byte, nblocks int)

// sha256FinishNI pads and hashes the tail p[full:] into state, then writes
// the digest.
func sha256FinishNI(state *[8]uint32, p []byte, full int, out *[32]byte) {
	var tail [128]byte
	rem := copy(tail[:], p[full:])
	tail[rem] = 0x80
	tlen := 64
	if rem >= 56 {
		tlen = 128
	}
	binary.BigEndian.PutUint64(tail[tlen-8:], uint64(len(p))<<3)
	sha256BlocksNI(state, &tail[0], tlen>>6)
	for i, s := range state {
		binary.BigEndian.PutUint32(out[i<<2:], s)
	}
}

// sha256Sum256Pair hashes two independent messages, sharing the 2-way block
// loop for their common-length prefix. Requires len(a) > 0 and len(b) > 0.
func sha256Sum256Pair(a, b []byte) (ha, hb [32]byte) {
	if !useSHANI {
		return sha256Fallback(a), sha256Fallback(b)
	}
	st0, st1 := sha256IV, sha256IV
	fa, fb := len(a)&^63, len(b)&^63
	shared := min(fa, fb) >> 6
	if shared > 0 {
		sha256Blocks2NI(&st0, &a[0], &st1, &b[0], shared)
	}
	if n := fa>>6 - shared; n > 0 {
		sha256BlocksNI(&st0, &a[shared<<6], n)
	}
	if n := fb>>6 - shared; n > 0 {
		sha256BlocksNI(&st1, &b[shared<<6], n)
	}
	sha256FinishNI(&st0, a, fa, &ha)
	sha256FinishNI(&st1, b, fb, &hb)
	return ha, hb
}

// pairHashAvailable reports whether HashPair actually batches on this host.
const pairHashPossible = true

func pairHashAvailable() bool { return useSHANI && LittleEndian }
