//go:build !stagestats

package astrobwt

// StageStatsEnabled reports whether this binary was built with -tags stagestats.
const StageStatsEnabled = false

func stageMark() uint64                    { return 0 }
func stageLap(int, uint64) uint64          { return 0 }
func stageLapN(int, uint64, uint64) uint64 { return 0 }

// StageSnapshot returns zeros unless built with -tags stagestats.
func StageSnapshot() (prologue, wolf, sa, sha, n uint64) { return }
