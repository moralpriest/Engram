//go:build !v114stats

package astrobwt

import "io"

// V114StatsEnabled reports whether v114 descriptor counters are compiled in.
const V114StatsEnabled = false

func v114StatsRecordGroup(uint32)                  {}
func v114StatsRecordLiteralGroup(int)              {}
func v114StatsRecordTwoRunMerge()                  {}
func v114StatsRecordLargeFallbackMerge()           {}
func v114StatsRecordGenericKeyColumn(bool)         {}
func v114StatsRecordGenericPredecessorColumn(bool) {}

func v114Cycles() uint64                                  { return 0 }
func v114StatsRecordMergeBracket(uint64)                  {}
func v114StatsRecordLiteralCompares(int, int)             {}
func v114StatsRecordTwoRunCompares(int)                   {}
func v114StatsRecordKWayCompares(int)                     {}
func v114StatsAnalyzeBigGroup(*stage4View, []uint32, int) {}

// PrintV114Stats is a no-op unless built with -tags v114stats.
func PrintV114Stats(io.Writer) {}
