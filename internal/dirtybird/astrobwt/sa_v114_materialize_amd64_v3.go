//go:build amd64.v3

package astrobwt

const uniqueRunBatchAvailable = true

//go:noescape
func materializeOrigins(dst, src *uint32, count, rel uint32)

//go:noescape
func writeUniqueRunBatch(sa *uint32, runs *stage5Run, arena *uint32, n, groupStart, outPos, logicalLen, arenaCap int) (int, int)
