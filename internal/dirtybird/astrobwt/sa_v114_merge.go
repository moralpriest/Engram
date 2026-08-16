package astrobwt

// Stage-5 merge: radix-sort the emitted records by 3-byte key, then write
// positions per equal-key group — singleton runs copy straight out,
// all-literal groups (<=32) insertion-sort on the stack, 2-run groups do a
// linear merge, and rare larger groups fall back to a bottom-up k-way merge.
// Port of write_fused_runs_to_sa and callees (v114_stubs.cpp).

import (
	"bytes"
	"math/bits"
	"unsafe"
)

// compareSuffixesAfterKey compares two suffixes whose first 3 bytes (the
// record key) are already known equal.
func compareSuffixesAfterKey(v *stage4View, a, b uint32) int {
	if a == b {
		return 0
	}
	aLen := v.logicalLen - a
	bLen := v.logicalLen - b
	commonWithKey := aLen
	if bLen < commonWithKey {
		commonWithKey = bLen
	}
	if commonWithKey <= 3 {
		if aLen == bLen {
			return 0
		}
		if aLen < bLen {
			return -1
		}
		return 1
	}

	common := commonWithKey - 3
	if common >= 8 {
		// The eight bytes after the shared key, read as one big-endian word
		// through raw pointers: no per-call slice header, no bounds check.
		// In bounds because common>=8 => min(aLen,bLen)>=11, so a+10 and b+10
		// stay below logicalLen<=len(data). BSWAP makes the native LE load a
		// lexicographic compare, exactly binary.BigEndian.Uint64.
		dp := unsafe.Pointer(&v.data[0])
		av := bits.ReverseBytes64(*(*uint64)(unsafe.Add(dp, uintptr(a)+3)))
		bv := bits.ReverseBytes64(*(*uint64)(unsafe.Add(dp, uintptr(b)+3)))
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
		if c := bytes.Compare(v.data[a+11:a+3+common], v.data[b+11:b+3+common]); c != 0 {
			return c
		}
	} else if c := bytes.Compare(v.data[a+3:a+3+common], v.data[b+3:b+3+common]); c != 0 {
		return c
	}
	if aLen == bLen {
		return 0
	}
	if aLen < bLen {
		return -1
	}
	return 1
}

func suffixLessAfterKey(v *stage4View, a, b uint32) bool {
	cmp := compareSuffixesAfterKey(v, a, b)
	if cmp != 0 {
		return cmp < 0
	}
	return a < b
}

// radixSortRunsByStoredKey sorts native little-endian 24-bit keys by running
// stable passes from the lexical last byte to the first. Ping-pongs
// runs<->tmp; the result lands in runs.
func radixSortRunsByStoredKey(v *v114Scratch) {
	runs := v.runs
	n := len(runs)
	if n <= 1 {
		return
	}
	tmp := v.radixTmp[:n]

	var counts0, counts1, counts2 [256]uint32
	for i := range runs {
		counts0[runs[i].key&0xff]++
		counts1[(runs[i].key>>8)&0xff]++
		counts2[(runs[i].key>>16)&0xff]++
	}

	// NOTE: raw-pointer scatter writes here measured NULL (kata-7, and the
	// prior ledger entry): this loop is memory-latency-bound on the random
	// scatter, so the bounds check hides behind the cache miss. Kept checked.
	var sum uint32
	for i := 0; i < 256; i++ {
		c := counts2[i]
		counts2[i] = sum
		sum += c
	}
	for i := range runs {
		r := runs[i]
		tmp[counts2[(r.key>>16)&0xff]] = r
		counts2[(r.key>>16)&0xff]++
	}

	sum = 0
	for i := 0; i < 256; i++ {
		c := counts1[i]
		counts1[i] = sum
		sum += c
	}
	for i := range tmp {
		r := tmp[i]
		runs[counts1[(r.key>>8)&0xff]] = r
		counts1[(r.key>>8)&0xff]++
	}

	sum = 0
	for i := 0; i < 256; i++ {
		c := counts0[i]
		counts0[i] = sum
		sum += c
	}
	for i := range runs {
		r := runs[i]
		tmp[counts0[r.key&0xff]] = r
		counts0[r.key&0xff]++
	}
	// result is in tmp: swap the buffers (runs keeps len n, radixTmp cap)
	v.runs = tmp
	v.radixTmp = runs[:cap(runs)]
}

func fusedRunPos(arena []uint32, r stage5Run, rel uint32) uint32 {
	begin := r.begin()
	if r.isLiteral() {
		return begin
	}
	return arena[begin+rel]
}

// constantRunOrder orders an all-literal equal-key group by the closed form
// available when the group's shared 3-byte key is one repeated byte c. Every
// member p then sits inside a maximal run of c's ending at e (data[e+1] != c
// or the run reaches logicalLen), and its suffix is c^(e-p+1) followed by
// the bytes after the run. For members p1 < p2 of the SAME run the first
// differing offset is e-p2+1, where suffix(p2) shows t = data[e+1] while
// suffix(p1) still shows c (p1+(e-p2+1) <= e because p1 < p2): t > c orders
// the run ascending by position, t < c descending, and t never equals c by
// maximality. A run reaching logicalLen makes the shorter suffix a proper
// prefix of the longer, so larger positions sort first — descending, the
// same as t < c. Group members of one run are exactly the positions
// [runStart, e-2] (any gap position carries the same key and would be a
// member, and groups over 32 members never reach the literal path), so
// delta-1 chains in position order are complete runs; distinct chains are
// distinct runs and are merged with real suffix compares. Returns false
// with the group untouched when the key is not a repeated byte.
func constantRunOrder(view *stage4View, positions []uint32) bool {
	n := len(positions)
	if n > 32 { // the literal path never exceeds 32; guard the stack arrays
		return false
	}
	c := view.data[positions[0]]
	if view.data[positions[0]+1] != c || view.data[positions[0]+2] != c {
		return false
	}

	var byPos [32]uint32
	sorted := byPos[:n]
	copy(sorted, positions)
	for i := 1; i < n; i++ { // position order: cheap uint32 compares
		p := sorted[i]
		j := i
		for j > 0 && sorted[j-1] > p {
			sorted[j] = sorted[j-1]
			j--
		}
		sorted[j] = p
	}

	var partStart [32]int
	k := 0
	for i := 0; i < n; i++ {
		if i == 0 || sorted[i] != sorted[i-1]+1 {
			partStart[k] = i
			k++
		}
	}

	var ordered, tmp [32]uint32
	for pi := 0; pi < k; pi++ {
		lo := partStart[pi]
		hi := n
		if pi+1 < k {
			hi = partStart[pi+1]
		}
		m := sorted[hi-1]
		e := m + 2 // the key guarantees c at m+1 and m+2
		for e+1 < view.logicalLen && view.data[e+1] == c {
			e++
		}
		if e+1 < view.logicalLen && view.data[e+1] > c {
			copy(ordered[lo:hi], sorted[lo:hi])
		} else {
			for i := lo; i < hi; i++ {
				ordered[i] = sorted[lo+hi-1-i]
			}
		}
	}

	if k > 1 {
		// merge the per-run segments (k is small) with real suffix compares
		var lens [32]int
		for pi := 0; pi < k; pi++ {
			hi := n
			if pi+1 < k {
				hi = partStart[pi+1]
			}
			lens[pi] = hi - partStart[pi]
		}
		src, dst := ordered[:n], tmp[:n]
		dummy := 0
		for k > 1 {
			w, base := 0, 0
			for i := 0; i < k; i += 2 {
				if i+1 == k {
					copy(dst[base:base+lens[i]], src[base:base+lens[i]])
					lens[w] = lens[i]
					w++
					break
				}
				l, r := lens[i], lens[i+1]
				mergeSortedPositionsAfterKey(view, src, base, base+l, base+l+r, dst, base, &dummy)
				lens[w] = l + r
				w++
				base += l + r
			}
			k = w
			src, dst = dst, src
		}
		copy(positions, src)
		return true
	}
	copy(positions, ordered[:n])
	return true
}

// tryWriteLiteralGroup handles equal-key groups of <=32 all-literal runs with
// a stack insertion sort.
func tryWriteLiteralGroup(view *stage4View, runs []stage5Run, sa []int32, outPos int) (int, bool) {
	count := len(runs)
	if count == 0 || count > 32 {
		return outPos, false
	}
	var positions [32]uint32
	for i := 0; i < count; i++ {
		if !runs[i].isLiteral() {
			return outPos, false
		}
		positions[i] = runs[i].begin()
	}
	v114StatsRecordLiteralGroup(count)
	if V114StatsEnabled && count >= 17 {
		v114StatsAnalyzeBigGroup(view, positions[:count], 0)
	}
	cmps := 0
	if count < 17 || !constantRunOrder(view, positions[:count]) {
		// small groups, and the rare large group whose key is not a
		// repeated byte, take the general insertion sort
		for i := 1; i < count; i++ {
			pos := positions[i]
			j := i
			for j > 0 {
				if V114StatsEnabled {
					cmps++
				}
				if !suffixLessAfterKey(view, pos, positions[j-1]) {
					break
				}
				positions[j] = positions[j-1]
				j--
			}
			positions[j] = pos
		}
	}
	if V114StatsEnabled {
		v114StatsRecordLiteralCompares(count, cmps)
	}
	for i := 0; i < count; i++ {
		sa[outPos] = int32(positions[i])
		outPos++
	}
	return outPos, true
}

// tryWriteTwoRuns merges exactly two runs linearly.
func tryWriteTwoRuns(view *stage4View, arena []uint32, runs []stage5Run, sa []int32, outPos int) (int, bool) {
	if len(runs) != 2 {
		return outPos, false
	}
	v114StatsRecordTwoRunMerge()
	left, right := runs[0], runs[1]
	leftCount, rightCount := left.count(), right.count()
	var leftRel, rightRel uint32
	cmps := 0
	for leftRel < leftCount && rightRel < rightCount {
		if V114StatsEnabled {
			cmps++
		}
		lpos := fusedRunPos(arena, left, leftRel)
		rpos := fusedRunPos(arena, right, rightRel)
		if suffixLessAfterKey(view, lpos, rpos) {
			sa[outPos] = int32(lpos)
			leftRel++
		} else {
			sa[outPos] = int32(rpos)
			rightRel++
		}
		outPos++
	}
	for leftRel < leftCount {
		sa[outPos] = int32(fusedRunPos(arena, left, leftRel))
		leftRel++
		outPos++
	}
	for rightRel < rightCount {
		sa[outPos] = int32(fusedRunPos(arena, right, rightRel))
		rightRel++
		outPos++
	}
	if V114StatsEnabled {
		v114StatsRecordTwoRunCompares(cmps)
	}
	return outPos, true
}

func mergeSortedPositionsAfterKey(view *stage4View, src []uint32, leftBegin, leftEnd, rightEnd int, dst []uint32, dstBegin int, cmps *int) {
	// Raw-pointer reads/writes: left<leftEnd<=len(src), right<rightEnd<=len(src),
	// and out spans dstBegin..dstBegin+(rightEnd-leftBegin)-1 < len(dst) by the
	// caller's sizing — all provable, none proven by gc, so drop the checks.
	// src/dst are the MAX_LENGTH scratch buffers, always non-empty.
	sp := unsafe.Pointer(&src[0])
	dp := unsafe.Pointer(&dst[0])
	left, right, out := leftBegin, leftEnd, dstBegin
	for left < leftEnd && right < rightEnd {
		if V114StatsEnabled {
			*cmps++
		}
		lpos := *(*uint32)(unsafe.Add(sp, uintptr(left)*4))
		rpos := *(*uint32)(unsafe.Add(sp, uintptr(right)*4))
		if suffixLessAfterKey(view, lpos, rpos) {
			*(*uint32)(unsafe.Add(dp, uintptr(out)*4)) = lpos
			left++
		} else {
			*(*uint32)(unsafe.Add(dp, uintptr(out)*4)) = rpos
			right++
		}
		out++
	}
	for left < leftEnd {
		*(*uint32)(unsafe.Add(dp, uintptr(out)*4)) = *(*uint32)(unsafe.Add(sp, uintptr(left)*4))
		left++
		out++
	}
	for right < rightEnd {
		*(*uint32)(unsafe.Add(dp, uintptr(out)*4)) = *(*uint32)(unsafe.Add(sp, uintptr(right)*4))
		right++
		out++
	}
}

// mergeEqualKeyRuns: bottom-up pairwise merge of the per-run sorted position
// lists in v.groupPos (lengths in v.runLens); result ends in v.groupPos.
func mergeEqualKeyRuns(view *stage4View, v *v114Scratch) {
	if len(v.runLens) <= 1 {
		return
	}
	cmps := 0
	n := len(v.groupPos)
	v.mergePos = v.mergePos[:cap(v.mergePos)]
	src := v.groupPos
	dst := v.mergePos[:n]
	fromGroupPos := true
	for len(v.runLens) > 1 {
		v.nextLens = v.nextLens[:0]
		inBase, outBase := 0, 0
		for i := 0; i < len(v.runLens); i += 2 {
			leftLen := int(v.runLens[i])
			if i+1 == len(v.runLens) {
				copy(dst[outBase:outBase+leftLen], src[inBase:inBase+leftLen])
				v.nextLens = append(v.nextLens, uint32(leftLen))
				inBase += leftLen
				outBase += leftLen
				continue
			}
			rightLen := int(v.runLens[i+1])
			mergeSortedPositionsAfterKey(view, src, inBase, inBase+leftLen, inBase+leftLen+rightLen, dst, outBase, &cmps)
			v.nextLens = append(v.nextLens, uint32(leftLen+rightLen))
			inBase += leftLen + rightLen
			outBase += leftLen + rightLen
		}
		v.runLens, v.nextLens = v.nextLens, v.runLens
		src, dst = dst, src
		fromGroupPos = !fromGroupPos
	}
	if !fromGroupPos { // final result sits in mergePos; move it back
		copy(v.groupPos[:n], src[:n])
	}
	if V114StatsEnabled {
		v114StatsRecordKWayCompares(cmps)
	}
}

// writeFusedRunsToSA sorts the records and writes the final SA positions.
func writeFusedRunsToSA(view *stage4View, v *v114Scratch, sa []int32) bool {
	radixSortRunsByStoredKey(v)
	logicalLen := len(sa)

	// uint32 view of sa: positions < 2^31, so int32/uint32 bits are identical
	// and arena runs can be bulk-copied (the C++ memcpys here). buildSAv114
	// guarantees len(sa) >= 1.
	saU32 := unsafe.Slice((*uint32)(unsafe.Pointer(&sa[0])), cap(sa))

	runs := v.runs
	arena := v.arena
	n := len(runs)
	saPtr := unsafe.SliceData(saU32)
	runsPtr := unsafe.SliceData(runs)
	arenaPtr := unsafe.SliceData(arena)
	groupStart := 0
	outPos := 0
	for groupStart < n {
		if uniqueRunBatchAvailable {
			groupStart, outPos = writeUniqueRunBatch(saPtr, runsPtr, arenaPtr, n, groupStart, outPos, logicalLen, cap(arena))
			if groupStart == n {
				break
			}
		}
		r0 := runs[groupStart]
		groupEnd := groupStart + 1
		for groupEnd < n && runs[groupEnd].key == r0.key {
			groupEnd++
		}

		if groupEnd == groupStart+1 {
			if r0.packed>>17 == 0 {
				// literal singleton (packed IS the position) — hottest case
				if outPos >= len(saU32) {
					return false
				}
				saU32[outPos] = r0.packed
				outPos++
			} else {
				begin := r0.begin()
				count := r0.count()
				if outPos+int(count) > logicalLen {
					return false
				}
				if count <= 8 && outPos+8 <= len(saU32) && int(begin)+8 <= cap(arena) {
					dst := unsafe.Pointer(&saU32[outPos])
					src := unsafe.Pointer(&arena[begin])
					*(*uint64)(dst) = *(*uint64)(src)
					*(*uint64)(unsafe.Add(dst, 8)) = *(*uint64)(unsafe.Add(src, 8))
					*(*uint64)(unsafe.Add(dst, 16)) = *(*uint64)(unsafe.Add(src, 16))
					*(*uint64)(unsafe.Add(dst, 24)) = *(*uint64)(unsafe.Add(src, 24))
					outPos += int(count)
				} else {
					outPos += copy(saU32[outPos:], arena[begin:begin+count])
				}
			}
		} else {
			var t0 uint64
			if V114StatsEnabled {
				t0 = v114Cycles()
			}
			group := runs[groupStart:groupEnd]
			var handled bool
			if outPos, handled = tryWriteLiteralGroup(view, group, sa, outPos); !handled {
				if outPos, handled = tryWriteTwoRuns(view, arena, group, sa, outPos); !handled {
					// rare fallback: expand all positions and k-way merge
					v114StatsRecordLargeFallbackMerge()
					v.groupPos = v.groupPos[:0]
					v.runLens = v.runLens[:0]
					for i := range group {
						count := group[i].count()
						v.runLens = append(v.runLens, count)
						for rel := uint32(0); rel < count; rel++ {
							v.groupPos = append(v.groupPos, fusedRunPos(arena, group[i], rel))
						}
					}
					if V114StatsEnabled && len(v.runLens) >= 2 {
						v114StatsAnalyzeBigGroup(view, v.groupPos, 1)
					}
					mergeEqualKeyRuns(view, v)
					if outPos+len(v.groupPos) > len(saU32) {
						return false
					}
					outPos += copy(saU32[outPos:], v.groupPos)
				}
			}
			if V114StatsEnabled {
				v114StatsRecordMergeBracket(v114Cycles() - t0)
			}
		}
		groupStart = groupEnd
	}

	return outPos == logicalLen
}
