//go:build stagestats

package astrobwt

import "sync/atomic"

// StageStatsEnabled reports whether this binary was built with -tags stagestats.
const StageStatsEnabled = true

var (
	stageCycles [stageCount]atomic.Uint64
	stageHashes atomic.Uint64
)

func stageMark() uint64 { return rdtsc() }

// stageLap accumulates t1-t0 into the stage counter and returns t1 so laps
// chain without re-reading the TSC.
func stageLap(stage int, t0 uint64) uint64 {
	return stageLapN(stage, t0, 1)
}

func stageLapN(stage int, t0, hashes uint64) uint64 {
	t1 := rdtsc()
	stageCycles[stage].Add(t1 - t0)
	if stage == stageSHA {
		stageHashes.Add(hashes)
	}
	return t1
}

// StageSnapshot returns cumulative per-stage cycles and the hash count.
func StageSnapshot() (prologue, wolf, sa, sha, n uint64) {
	return stageCycles[stagePrologue].Load(), stageCycles[stageWolf].Load(),
		stageCycles[stageSA].Load(), stageCycles[stageSHA].Load(), stageHashes.Load()
}
