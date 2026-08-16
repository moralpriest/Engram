//go:build stagestats && amd64

package astrobwt

// rdtsc returns the CPU timestamp counter. Unserialized on purpose: the
// counters only ever report cumulative sums over thousands of hashes, so
// per-read reorder slop cancels out.
func rdtsc() uint64
