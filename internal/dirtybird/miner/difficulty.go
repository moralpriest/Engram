package miner

// Difficulty semantics ported from derohe cmd/dero-miner/difficulty.go:
// a hash passes iff its byte-reversed value, read as a big-endian integer,
// is <= (1<<256)/difficulty. The big.Int division happens once per job
// (ComputeTarget); the per-hash check (MeetsTarget) is an alloc-free
// byte compare.

import "math/big"

var oneLsh256 = new(big.Int).Lsh(big.NewInt(1), 256)

// ComputeTarget returns (1<<256)/diff as 32 big-endian bytes.
func ComputeTarget(diff uint64) (target [32]byte) {
	if diff == 0 {
		return // zero target: nothing ever passes; callers reject diff==0 jobs
	}
	if diff == 1 {
		// (1<<256)/1 needs 257 bits; every 256-bit hash passes, and an
		// all-ones target accepts everything under the <= compare.
		for i := range target {
			target[i] = 0xff
		}
		return
	}
	q := new(big.Int).Div(oneLsh256, new(big.Int).SetUint64(diff))
	q.FillBytes(target[:])
	return
}

// MeetsTarget reports whether pow (little-endian, as produced by AstroBWTv3)
// reversed and read big-endian is <= target.
func MeetsTarget(pow *[32]byte, target *[32]byte) bool {
	for i := 0; i < 32; i++ {
		p := pow[31-i]
		t := target[i]
		if p < t {
			return true
		}
		if p > t {
			return false
		}
	}
	return true // exactly equal passes (<=)
}
