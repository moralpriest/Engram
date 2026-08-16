package astrobwt

// Backend selects the suffix-array implementation for the ~90%-of-runtime SA
// stage. Both produce identical hashes; V114 is ~2x faster and falls back to
// SAIS per-hash whenever it declines an input.
type Backend int

const (
	BackendSAIS Backend = iota // Go-stdlib SAIS (the derohe reference path)
	BackendV114                // v1.14 descriptor SA (structure-aware, fused)
)

// Hasher computes DERO AstroBWTv3 hashes with a persistent, caller-owned
// scratch buffer. Not safe for concurrent use: one Hasher per worker.
// scratch2 exists only after the first HashPair call. Pair mode duplicates
// only the per-lane stream state (the two suffix arrays must coexist for the
// batched final SHA); the v114 SA working scratch is lane-transient — each
// build fully rewrites it before reading — so both lanes share one, the same
// split the zig miner uses (per-lane Workers, one shared SA scratch).
type Hasher struct {
	scratch  *ScratchData
	scratch2 *ScratchData
}

func New() *Hasher {
	return NewWithBackend(BackendSAIS)
}

func NewWithBackend(b Backend) *Hasher {
	h := &Hasher{scratch: NewScratchData()}
	if b == BackendV114 {
		h.scratch.useV114 = true
		h.scratch.v114 = newV114Scratch()
	}
	return h
}

func (h *Hasher) Hash(input []byte) [32]byte {
	return astroBWTv3(input, h.scratch)
}

// HashPair hashes two independent inputs, running their final SHA-256 as one
// 2-way SHA-NI batch (single-stream SHA-NI is latency-bound; interleaving two
// nonces recovers ~1.3x SHA throughput). Falls back to two sequential hashes
// when the host lacks SHA-NI or is big-endian.
func (h *Hasher) HashPair(a, b []byte) (ha, hb [32]byte) {
	if !pairHashAvailable() {
		return h.Hash(a), h.Hash(b)
	}
	if h.scratch2 == nil {
		h.scratch2 = NewScratchData()
		h.scratch2.useV114 = h.scratch.useV114
	}
	la := astroBWTv3Stream(a, h.scratch)
	// Lane B starts only after lane A's suffix array is fully written to
	// scratch.sa_bytes, so the SA working scratch never carries live state
	// between lanes; sharing it halves the pair-mode working set. Assigned
	// after lane A because the first build may allocate it lazily.
	h.scratch2.v114 = h.scratch.v114
	lb := astroBWTv3Stream(b, h.scratch2)
	ts := stageMark()
	ha, hb = sha256Sum256Pair(h.scratch.sa_bytes[:la*4], h.scratch2.sa_bytes[:lb*4])
	stageLapN(stageSHA, ts, 2)
	return ha, hb
}

// PairHashSupported reports whether HashPair actually batches on this host
// (multi-buffer SHA kernel present and usable). Callers use it to pick the
// -pair default: on when batching wins by default (arm64), opt-in elsewhere.
func PairHashSupported() bool { return pairHashAvailable() }

// Sum is a convenience for one-off hashing (KAT, selftest); it allocates a
// fresh scratch every call, so miners must use a per-worker Hasher instead.
func Sum(input []byte) [32]byte {
	return astroBWTv3(input, NewScratchData())
}
