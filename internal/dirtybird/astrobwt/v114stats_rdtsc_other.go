//go:build v114stats && !amd64

package astrobwt

// No cycle counter wired up on this architecture: the merge-share bracket
// reports zero and PrintV114Stats omits the cycle line.
func v114rdtsc() uint64 { return 0 }
