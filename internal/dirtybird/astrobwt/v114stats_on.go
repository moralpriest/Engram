//go:build v114stats

package astrobwt

import (
	"fmt"
	"io"
	"sync/atomic"
)

// V114StatsEnabled reports whether v114 descriptor counters are compiled in.
const V114StatsEnabled = true

const (
	v114Group1 = iota
	v114Group2
	v114Group3
	v114Group4
	v114Group5To8
	v114Group9To16
	v114Group17To25
	v114Group26Plus
	v114GroupBucketCount
)

const (
	v114Literal1 = iota
	v114Literal2To4
	v114Literal5To8
	v114Literal9To16
	v114Literal17To32
	v114LiteralBucketCount
)

var (
	v114GroupRuns           [v114GroupBucketCount]atomic.Uint64
	v114LiteralMergeGroups  [v114LiteralBucketCount]atomic.Uint64
	v114TwoRunMerges        atomic.Uint64
	v114LargeFallbackMerges atomic.Uint64
	v114GenericKeyColumns   atomic.Uint64
	v114EqualKeyColumns     atomic.Uint64
	v114GenericPredColumns  atomic.Uint64
	v114EqualPredColumns    atomic.Uint64

	// comparator campaign counters: suffix compares per merge path, compare
	// totals per literal bucket, and the RDTSC bracket over the whole
	// multi-record merge arm
	v114LiteralCompares       atomic.Uint64
	v114TwoRunCompares        atomic.Uint64
	v114KWayCompares          atomic.Uint64
	v114LiteralBucketCompares [v114LiteralBucketCount]atomic.Uint64
	v114MergeCycles           atomic.Uint64
	v114MergeBrackets         atomic.Uint64

	// low-entropy discriminator for large groups, indexed 0 = literal
	// count>=17, 1 = k-way fallback
	v114BigGroups       [2]atomic.Uint64
	v114BigGroupsVVV    [2]atomic.Uint64
	v114BigGroupsContig [2]atomic.Uint64
	v114BigGroupLCP     [2][6]atomic.Uint64 // 0-7,8-15,16-31,32-63,64-127,128+
)

// v114BracketCost is the median cost of one empty RDTSC pair, measured at
// init and subtracted per bracket at print time (~331 brackets/hash would
// otherwise inflate the merge share by ~1-2%).
var v114BracketCost = func() uint64 {
	var samples [31]uint64
	for i := range samples {
		t0 := v114rdtsc()
		samples[i] = v114rdtsc() - t0
	}
	for i := 1; i < len(samples); i++ { // insertion sort, tiny fixed n
		v := samples[i]
		j := i
		for j > 0 && samples[j-1] > v {
			samples[j] = samples[j-1]
			j--
		}
		samples[j] = v
	}
	return samples[15]
}()

func v114Cycles() uint64 { return v114rdtsc() }

func v114LiteralBucketFor(count int) int {
	switch {
	case count <= 1:
		return v114Literal1
	case count <= 4:
		return v114Literal2To4
	case count <= 8:
		return v114Literal5To8
	case count <= 16:
		return v114Literal9To16
	default:
		return v114Literal17To32
	}
}

func v114StatsRecordLiteralCompares(count, cmps int) {
	v114LiteralCompares.Add(uint64(cmps))
	v114LiteralBucketCompares[v114LiteralBucketFor(count)].Add(uint64(cmps))
}

func v114StatsRecordTwoRunCompares(cmps int) { v114TwoRunCompares.Add(uint64(cmps)) }
func v114StatsRecordKWayCompares(cmps int)   { v114KWayCompares.Add(uint64(cmps)) }

func v114StatsRecordMergeBracket(cycles uint64) {
	v114MergeCycles.Add(cycles)
	v114MergeBrackets.Add(1)
}

// v114StatsAnalyzeBigGroup records the low-entropy discriminator for one
// large equal-key group: whether the shared 3-byte key is a single repeated
// byte (the constant-run signature), how position-contiguous the members
// are, and the LCP between the extreme members. path 0 = literal count>=17,
// path 1 = k-way fallback.
func v114StatsAnalyzeBigGroup(view *stage4View, positions []uint32, path int) {
	n := len(positions)
	if n < 2 {
		return
	}
	v114BigGroups[path].Add(1)

	key0 := view.data[positions[0]]
	vvv := view.data[positions[0]+1] == key0 && view.data[positions[0]+2] == key0
	if vvv {
		v114BigGroupsVVV[path].Add(1)
	}

	// sort a bounded copy by position; contiguity = share of adjacent
	// deltas equal to 1
	var buf [64]uint32
	m := n
	if m > len(buf) {
		m = len(buf)
	}
	sorted := buf[:m]
	copy(sorted, positions[:m])
	for i := 1; i < m; i++ {
		p := sorted[i]
		j := i
		for j > 0 && sorted[j-1] > p {
			sorted[j] = sorted[j-1]
			j--
		}
		sorted[j] = p
	}
	adjacent := 0
	for i := 1; i < m; i++ {
		if sorted[i]-sorted[i-1] == 1 {
			adjacent++
		}
	}
	if vvv && adjacent*10 >= (m-1)*8 { // >= 80% delta-1 chains
		v114BigGroupsContig[path].Add(1)
	}

	// LCP between the extreme members, bucketed
	a, b := sorted[0], sorted[m-1]
	limit := view.logicalLen - b // the shorter suffix bounds the walk
	lcp := uint32(0)
	for lcp < limit && view.data[a+lcp] == view.data[b+lcp] {
		lcp++
	}
	bucket := 0
	switch {
	case lcp < 8:
		bucket = 0
	case lcp < 16:
		bucket = 1
	case lcp < 32:
		bucket = 2
	case lcp < 64:
		bucket = 3
	case lcp < 128:
		bucket = 4
	default:
		bucket = 5
	}
	v114BigGroupLCP[path][bucket].Add(1)
}

func v114StatsRecordGroup(groupCount uint32) {
	switch {
	case groupCount == 1:
		v114GroupRuns[v114Group1].Add(1)
	case groupCount == 2:
		v114GroupRuns[v114Group2].Add(1)
	case groupCount == 3:
		v114GroupRuns[v114Group3].Add(1)
	case groupCount == 4:
		v114GroupRuns[v114Group4].Add(1)
	case groupCount <= 8:
		v114GroupRuns[v114Group5To8].Add(1)
	case groupCount <= 16:
		v114GroupRuns[v114Group9To16].Add(1)
	case groupCount <= stage4ShortRunMax:
		v114GroupRuns[v114Group17To25].Add(1)
	default:
		v114GroupRuns[v114Group26Plus].Add(1)
	}
}

func v114StatsRecordLiteralGroup(count int) {
	switch {
	case count <= 1:
		v114LiteralMergeGroups[v114Literal1].Add(1)
	case count <= 4:
		v114LiteralMergeGroups[v114Literal2To4].Add(1)
	case count <= 8:
		v114LiteralMergeGroups[v114Literal5To8].Add(1)
	case count <= 16:
		v114LiteralMergeGroups[v114Literal9To16].Add(1)
	default:
		v114LiteralMergeGroups[v114Literal17To32].Add(1)
	}
}

func v114StatsRecordTwoRunMerge() {
	v114TwoRunMerges.Add(1)
}

func v114StatsRecordLargeFallbackMerge() {
	v114LargeFallbackMerges.Add(1)
}

func v114StatsRecordGenericKeyColumn(equal bool) {
	v114GenericKeyColumns.Add(1)
	if equal {
		v114EqualKeyColumns.Add(1)
	}
}

func v114StatsRecordGenericPredecessorColumn(equal bool) {
	v114GenericPredColumns.Add(1)
	if equal {
		v114EqualPredColumns.Add(1)
	}
}

// PrintV114Stats prints cumulative descriptor counters for benchmark runs.
func PrintV114Stats(w io.Writer) {
	groupLabels := [...]string{"1", "2", "3", "4", "5-8", "9-16", "17-25", "26+"}
	literalLabels := [...]string{"1", "2-4", "5-8", "9-16", "17-32"}

	fmt.Fprintln(w, "\nv114 descriptor stats:")
	fmt.Fprintln(w, "  full-group run counts:")
	for i, label := range groupLabels {
		fmt.Fprintf(w, "    %6s groups: %d\n", label, v114GroupRuns[i].Load())
	}
	fmt.Fprintln(w, "  equal-key merge fast paths:")
	for i, label := range literalLabels {
		fmt.Fprintf(w, "    literal %5s: %d\n", label, v114LiteralMergeGroups[i].Load())
	}
	fmt.Fprintf(w, "    two-run merge: %d\n", v114TwoRunMerges.Load())
	fmt.Fprintf(w, "    large fallback merge: %d\n", v114LargeFallbackMerges.Load())
	fmt.Fprintf(w, "    v114 fallback hashes: %d\n", V114Fallbacks())
	if columns := v114GenericKeyColumns.Load(); columns > 0 {
		fmt.Fprintf(w, "    equal generic key columns: %d/%d (%.2f%%)\n",
			v114EqualKeyColumns.Load(), columns, 100*float64(v114EqualKeyColumns.Load())/float64(columns))
	}
	if columns := v114GenericPredColumns.Load(); columns > 0 {
		fmt.Fprintf(w, "    equal generic predecessor columns: %d/%d (%.2f%%)\n",
			v114EqualPredColumns.Load(), columns, 100*float64(v114EqualPredColumns.Load())/float64(columns))
	}

	fmt.Fprintln(w, "  comparator:")
	fmt.Fprintf(w, "    compares literal=%d two-run=%d k-way=%d\n",
		v114LiteralCompares.Load(), v114TwoRunCompares.Load(), v114KWayCompares.Load())
	literalLabels2 := [...]string{"1", "2-4", "5-8", "9-16", "17-32"}
	for i, label := range literalLabels2 {
		groups := v114LiteralMergeGroups[i].Load()
		if groups == 0 {
			continue
		}
		fmt.Fprintf(w, "    literal %5s: %.2f compares/group\n",
			label, float64(v114LiteralBucketCompares[i].Load())/float64(groups))
	}
	if brackets := v114MergeBrackets.Load(); brackets > 0 {
		raw := v114MergeCycles.Load()
		corrected := raw - brackets*v114BracketCost
		if corrected > raw { // underflow guard
			corrected = 0
		}
		fmt.Fprintf(w, "    merge-branch cycles: %d over %d brackets (bracket cost %d cyc, corrected %d)\n",
			raw, brackets, v114BracketCost, corrected)
	}
	for path, name := range [...]string{"literal>=17", "k-way"} {
		total := v114BigGroups[path].Load()
		if total == 0 {
			continue
		}
		fmt.Fprintf(w, "    %s groups: %d, vvv-key %d, contiguous %d, extreme-LCP buckets [<8 %d, <16 %d, <32 %d, <64 %d, <128 %d, 128+ %d]\n",
			name, total, v114BigGroupsVVV[path].Load(), v114BigGroupsContig[path].Load(),
			v114BigGroupLCP[path][0].Load(), v114BigGroupLCP[path][1].Load(),
			v114BigGroupLCP[path][2].Load(), v114BigGroupLCP[path][3].Load(),
			v114BigGroupLCP[path][4].Load(), v114BigGroupLCP[path][5].Load())
	}
}
