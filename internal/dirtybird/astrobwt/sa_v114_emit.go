package astrobwt

// Stage-4 emit: for each template run of k full 256-byte groups, sort the k
// column-255 suffixes outright, then induce columns 254..0 (decrement each
// position; stable first-byte insertion re-sort), emitting one record per
// (column x equal-3-byte-key group). Port of emit_full_group_run_compact_fused
// and callees (v114_stubs.cpp).

import (
	"bytes"
	"unsafe"
)

// compareSuffixes is the stage-4 suffix compare (shorter suffix wins on a
// common-prefix tie — exactly bytes.Compare on the two suffix slices, which
// runs as vectorized asm; the C++ analog is scalar). Wolf output is highly
// repetitive, so compares routinely scan long common prefixes.
func compareSuffixes(v *stage4View, a, b uint32) int {
	if a == b {
		return 0
	}
	return bytes.Compare(v.data[a:v.logicalLen], v.data[b:v.logicalLen])
}

// sortSuffixOrderSmall: insertion sort by full suffix compare (short runs).
func sortSuffixOrderSmall(v *stage4View, order []uint32) {
	for i := 1; i < len(order); i++ {
		pos := order[i]
		j := i
		for j > 0 && compareSuffixes(v, pos, order[j-1]) < 0 {
			order[j] = order[j-1]
			j--
		}
		order[j] = pos
	}
}

// heapSortSuffixOrder replaces the C++ std::sort for long runs without
// allocating (suffix compare is a strict total order on distinct positions).
func heapSortSuffixOrder(v *stage4View, order []uint32) {
	n := len(order)
	for i := n/2 - 1; i >= 0; i-- {
		siftDownSuffix(v, order, i, n)
	}
	for i := n - 1; i > 0; i-- {
		order[0], order[i] = order[i], order[0]
		siftDownSuffix(v, order, 0, i)
	}
}

func siftDownSuffix(v *stage4View, order []uint32, root, n int) {
	for {
		child := 2*root + 1
		if child >= n {
			return
		}
		if child+1 < n && compareSuffixes(v, order[child], order[child+1]) < 0 {
			child++
		}
		if compareSuffixes(v, order[root], order[child]) >= 0 {
			return
		}
		order[root], order[child] = order[child], order[root]
		root = child
	}
}

// appendOriginGroup materializes one suffix position per group origin at rel.
// Singletons stay literal; multi-position groups spill to the arena.
func appendOriginGroup(v *v114Scratch, key uint32, origins []uint32, first, count, rel uint32) bool {
	if count == 0 {
		return false
	}
	if count == 1 {
		v.runs = append(v.runs, stage5Run{key: key, packed: origins[first] + rel})
		return true
	}
	begin := uint32(len(v.arena))
	if begin > arenaIndexCount || count > arenaIndexCount-begin {
		return false
	}
	v.arena = v.arena[:begin+count]
	if count >= 4 {
		materializeOrigins(&v.arena[begin], &origins[first], count, rel)
	} else {
		for i := uint32(0); i < count; i++ {
			v.arena[begin+i] = origins[first+i] + rel
		}
	}
	v.runs = append(v.runs, stage5Run{key: key, packed: count<<17 + begin})
	return true
}

// emitLiteralRecords: one singleton record per position (single-group
// templates and the partial tail group). Appends records directly — no
// per-position appendOrderGroup call or stack array; unchecked 24-bit key
// loads (positions < logicalLen, 4 zeroed padding bytes past it).
func emitLiteralRecords(view *stage4View, start, count uint32, v *v114Scratch) bool {
	dp := unsafe.Pointer(&view.data[0])
	runs := v.runs
	for rel := uint32(0); rel < count; rel++ {
		pos := start + rel
		key := *(*uint32)(unsafe.Add(dp, pos)) & 0xffffff
		runs = append(runs, stage5Run{key: key, packed: pos})
	}
	v.runs = runs
	return true
}

// emitFullGroupRunTwo: the k==2 specialization, with the singleton/two-run
// appends inlined and unchecked key loads.
func emitFullGroupRunTwo(view *stage4View, startGroup uint32, v *v114Scratch) bool {
	base := startGroup << 8
	var order [2]uint32
	order[0], order[1] = base+255, base+511
	if compareSuffixes(view, order[1], order[0]) < 0 {
		order[0], order[1] = order[1], order[0]
	}

	dp := unsafe.Pointer(&view.data[0])
	for rel := 255; rel >= 0; rel-- {
		key0 := *(*uint32)(unsafe.Add(dp, order[0])) & 0xffffff
		key1 := *(*uint32)(unsafe.Add(dp, order[1])) & 0xffffff
		if key0 == key1 {
			begin := uint32(len(v.arena))
			if begin > arenaIndexCount || 2 > arenaIndexCount-begin {
				return false
			}
			v.arena = append(v.arena, order[0], order[1])
			v.runs = append(v.runs, stage5Run{key: key0, packed: 2<<17 + begin})
		} else {
			v.runs = append(v.runs,
				stage5Run{key: key0, packed: order[0]},
				stage5Run{key: key1, packed: order[1]})
		}
		if rel > 0 {
			order[0]--
			order[1]--
			if *(*byte)(unsafe.Add(dp, order[0])) > *(*byte)(unsafe.Add(dp, order[1])) {
				order[0], order[1] = order[1], order[0]
			}
		}
	}
	return true
}

// emitFullGroupRunGeneric covers k>=3: full sort of the column-255 suffixes,
// then per-column key grouping and stable first-byte induction. The C++
// splits this into short(<=25)/fixed<3,4>/general variants purely as manual
// specializations of the same algorithm; one parameterized loop is
// behavior-identical.
func emitFullGroupRunGeneric(view *stage4View, startGroup, groupCount uint32, v *v114Scratch) bool {
	base := startGroup << 8
	// int counters + len-capped slices: the prove pass can then discharge the
	// order[]/keys[] bounds checks in the per-column loops (one IsSliceInBounds
	// each here instead of checks all over the induction re-sort).
	gc := int(groupCount)
	order := v.order[:gc:gc]
	for chunk := 0; chunk < gc; chunk++ {
		order[chunk] = base + uint32(chunk)<<8 + 255
	}
	if groupCount <= stage4ShortRunMax {
		sortSuffixOrderSmall(view, order)
	} else {
		heapSortSuffixOrder(view, order)
	}
	for i := range order {
		order[i] -= 255
	}

	// All positions in order stay < logicalLen <= len(data)-4 throughout, so
	// the hot per-column loops read data through unchecked pointers (the
	// bounds checks here were ~15% of the SA stage).
	dp := unsafe.Pointer(&view.data[0])
	var equalColumns [4]uint64
	buildEqualColumns((*byte)(unsafe.Add(dp, base)), groupCount, &equalColumns)
	columnEqual := func(rel int) bool {
		return equalColumns[rel>>6]&(uint64(1)<<uint(rel&63)) != 0
	}

	for rel := 255; rel >= 0; rel-- {
		relu := uint32(rel)
		firstKey := *(*uint32)(unsafe.Add(dp, order[0]+relu)) & 0xffffff
		equalKey := rel <= 253 && columnEqual(rel) && columnEqual(rel+1) && columnEqual(rel+2)
		if rel > 253 {
			equalKey = firstKey == *(*uint32)(unsafe.Add(dp, order[gc-1]+relu))&0xffffff
		}
		v114StatsRecordGenericKeyColumn(equalKey)
		if equalKey {
			// groupCount >= 3 here. Keep the overwhelmingly common equal-column
			// append in this loop so the compiler can drop appendOriginGroup's
			// singleton checks and slice arguments.
			begin := uint32(len(v.arena))
			if begin > arenaIndexCount || groupCount > arenaIndexCount-begin {
				return false
			}
			v.arena = v.arena[:begin+groupCount]
			if groupCount >= 4 {
				materializeOrigins(&v.arena[begin], &order[0], groupCount, relu)
			} else {
				v.arena[begin] = order[0] + relu
				v.arena[begin+1] = order[1] + relu
				v.arena[begin+2] = order[2] + relu
			}
			v.runs = append(v.runs, stage5Run{key: firstKey, packed: groupCount<<17 + begin})
		} else {
			groupStart := 0
			for groupStart < gc {
				key := *(*uint32)(unsafe.Add(dp, order[groupStart]+relu)) & 0xffffff
				groupEnd := groupStart + 1
				for groupEnd < gc && *(*uint32)(unsafe.Add(dp, order[groupEnd]+relu))&0xffffff == key {
					groupEnd++
				}
				if groupEnd == groupStart+1 {
					// singleton fast path (most groups); avoids the call
					v.runs = append(v.runs, stage5Run{key: key, packed: order[groupStart] + relu})
				} else if !appendOriginGroup(v, key, order, uint32(groupStart), uint32(groupEnd-groupStart), relu) {
					return false
				}
				groupStart = groupEnd
			}
		}

		if rel > 0 {
			nextRel := uint32(rel - 1)
			equalPredecessor := columnEqual(rel - 1)
			v114StatsRecordGenericPredecessorColumn(equalPredecessor)
			if equalPredecessor {
				continue
			}

			// Stable first-byte induction over fixed group origins.
			for i := 1; i < gc; i++ {
				origin := order[i]
				b := *(*byte)(unsafe.Add(dp, origin+nextRel))
				j := i
				for j > 0 && *(*byte)(unsafe.Add(dp, order[j-1]+nextRel)) > b {
					order[j] = order[j-1]
					j--
				}
				order[j] = origin
			}
		}
	}
	return true
}

// emitFullGroupRun dispatches one template run [startGroup, endGroup).
func emitFullGroupRun(view *stage4View, startGroup, endGroup uint32, v *v114Scratch) bool {
	groupCount := endGroup - startGroup
	if groupCount == 0 {
		return true
	}
	if groupCount > stage4MaxGroupRun {
		return false
	}
	v114StatsRecordGroup(groupCount)
	if groupCount == 1 {
		return emitLiteralRecords(view, startGroup<<8, 256, v)
	}
	if groupCount == 2 {
		return emitFullGroupRunTwo(view, startGroup, v)
	}
	return emitFullGroupRunGeneric(view, startGroup, groupCount, v)
}
