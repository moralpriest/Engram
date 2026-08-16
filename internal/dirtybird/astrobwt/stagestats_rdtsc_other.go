//go:build stagestats && !amd64

package astrobwt

import "time"

var stageEpoch = time.Now()

// Non-amd64 fallback: nanoseconds, not cycles.
func rdtsc() uint64 { return uint64(time.Since(stageEpoch)) }
