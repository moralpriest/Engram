package astrobwt

//go:generate go run ./lutgen

// Lookup-table kernel for the branchy op loop. 149 of the 256 ops are a function
// of a single byte, so their four-op dependent chain (variable rotate, popcount,
// multiply) collapses to one table load. The remaining ops read step_3[pos1] or
// step_3[pos2], or carry hash-state prologues, and stay in the switch.
//
// The tables are ~37KB, which fits L1d on the P-cores. Building them per call
// could never pay: the op loop runs at most 32 iterations (pos2-pos1 is masked
// to 0x1f), so only a statically precomputed table can win.
//
// tnn-miner ships this as one of four kernels and auto-tunes between them per
// CPU, so the win is microarchitecture-dependent by construction; see useLUT.

// opLUT[opRow[op]][x] is the result of applying op to byte x.
var opLUT [pureOpCount][256]byte

// opRow maps an op to its row in opLUT, or -1 when the op is not LUT-able.
var opRow [256]int16

func init() {
	row := 0
	for op := 0; op < 256; op++ {
		if !isPureOp(byte(op)) {
			opRow[op] = -1
			continue
		}
		opRow[op] = int16(row)
		for x := 0; x < 256; x++ {
			opLUT[row][x] = pureOp(byte(op), byte(x))
		}
		row++
	}
	if row != pureOpCount {
		panic("astrobwt: pureOpSet disagrees with pureOpCount")
	}
}
