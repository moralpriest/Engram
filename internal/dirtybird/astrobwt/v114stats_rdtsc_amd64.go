//go:build v114stats && amd64

package astrobwt

// v114rdtsc reads the invariant TSC for the tagged-only merge-branch
// bracket. Deliberately separate from the stagestats reader so the two
// instrumentation tags stay independent.
func v114rdtsc() uint64
