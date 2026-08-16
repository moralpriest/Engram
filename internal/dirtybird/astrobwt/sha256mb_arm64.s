//go:build arm64

#include "textflag.h"

// SHA-256 block kernels on the ARMv8 crypto extensions: a single-stream
// version (tail/finish blocks) and a two-stream version whose instruction
// streams are interleaved so the two serially-dependent SHA256H chains
// overlap in the pipeline. Round structure follows the Go stdlib
// sha256block_arm64.s; the two-lane interleave mirrors the Rust sibling's
// sha256_x2 (+8.8%/hash on Neoverse-N2 — same instruction count, the win is
// entirely scheduling, which is why each round group keeps both lanes in one
// straight-line region).
//
// Lane A registers: V0,V1 running state; V2,V3 working state; V4-V7 message
// schedule; V9 wk; V15 abcd backup.
// Lane B registers: V20,V21; V22,V23; V24-V27; V29; V30.
// Shared: V8 holds the K constant for the current round group.

// One scheduled round group (rounds i..i+3 plus message extension), lane A
// only.
#define SCHED1(W0, W1, W2, W3) \
	VLD1.P	16(R4), [V8.S4]           \
	VADD	W0.S4, V8.S4, V9.S4       \
	SHA256SU0	W1.S4, W0.S4          \
	VMOV	V2.B16, V15.B16           \
	SHA256H	V9.S4, V3, V2             \
	SHA256H2	V9.S4, V15, V3        \
	SHA256SU1	W3.S4, W2.S4, W0.S4

// One final round group (no message extension), lane A only.
#define FINAL1(W0) \
	VLD1.P	16(R4), [V8.S4]      \
	VADD	W0.S4, V8.S4, V9.S4  \
	VMOV	V2.B16, V15.B16      \
	SHA256H	V9.S4, V3, V2        \
	SHA256H2	V9.S4, V15, V3

// Two-lane versions: identical dataflow per lane, instructions interleaved.
#define SCHED2(A0, A1, A2, A3, B0, B1, B2, B3) \
	VLD1.P	16(R4), [V8.S4]              \
	VADD	A0.S4, V8.S4, V9.S4          \
	VADD	B0.S4, V8.S4, V29.S4         \
	SHA256SU0	A1.S4, A0.S4             \
	SHA256SU0	B1.S4, B0.S4             \
	VMOV	V2.B16, V15.B16              \
	VMOV	V22.B16, V30.B16             \
	SHA256H	V9.S4, V3, V2                \
	SHA256H	V29.S4, V23, V22             \
	SHA256H2	V9.S4, V15, V3           \
	SHA256H2	V29.S4, V30, V23         \
	SHA256SU1	A3.S4, A2.S4, A0.S4      \
	SHA256SU1	B3.S4, B2.S4, B0.S4

#define FINAL2(A0, B0) \
	VLD1.P	16(R4), [V8.S4]      \
	VADD	A0.S4, V8.S4, V9.S4  \
	VADD	B0.S4, V8.S4, V29.S4 \
	VMOV	V2.B16, V15.B16      \
	VMOV	V22.B16, V30.B16     \
	SHA256H	V9.S4, V3, V2        \
	SHA256H	V29.S4, V23, V22     \
	SHA256H2	V9.S4, V15, V3   \
	SHA256H2	V29.S4, V30, V23

// func sha256BlocksARM64(state *[8]uint32, p *byte, nblocks int)
TEXT ·sha256BlocksARM64(SB), NOSPLIT, $0-24
	MOVD	state+0(FP), R0
	MOVD	p+8(FP), R1
	MOVD	nblocks+16(FP), R5
	CBZ	R5, done1 // do-while body would otherwise run ~2^64 blocks
	VLD1	(R0), [V0.S4, V1.S4]

loop1:
	MOVD	$·sha256KARM64(SB), R4
	VLD1.P	64(R1), [V4.B16, V5.B16, V6.B16, V7.B16]
	VREV32	V4.B16, V4.B16
	VREV32	V5.B16, V5.B16
	VREV32	V6.B16, V6.B16
	VREV32	V7.B16, V7.B16
	VMOV	V0.B16, V2.B16
	VMOV	V1.B16, V3.B16

	SCHED1(V4, V5, V6, V7)
	SCHED1(V5, V6, V7, V4)
	SCHED1(V6, V7, V4, V5)
	SCHED1(V7, V4, V5, V6)
	SCHED1(V4, V5, V6, V7)
	SCHED1(V5, V6, V7, V4)
	SCHED1(V6, V7, V4, V5)
	SCHED1(V7, V4, V5, V6)
	SCHED1(V4, V5, V6, V7)
	SCHED1(V5, V6, V7, V4)
	SCHED1(V6, V7, V4, V5)
	SCHED1(V7, V4, V5, V6)
	FINAL1(V4)
	FINAL1(V5)
	FINAL1(V6)
	FINAL1(V7)

	VADD	V2.S4, V0.S4, V0.S4
	VADD	V3.S4, V1.S4, V1.S4
	SUB	$1, R5, R5
	CBNZ	R5, loop1

	VST1	[V0.S4, V1.S4], (R0)
done1:
	RET

// func sha256Blocks2ARM64(state0 *[8]uint32, p0 *byte, state1 *[8]uint32, p1 *byte, nblocks int)
TEXT ·sha256Blocks2ARM64(SB), NOSPLIT, $0-40
	MOVD	state0+0(FP), R0
	MOVD	p0+8(FP), R1
	MOVD	state1+16(FP), R2
	MOVD	p1+24(FP), R3
	MOVD	nblocks+32(FP), R5
	CBZ	R5, done2 // do-while body would otherwise run ~2^64 blocks
	VLD1	(R0), [V0.S4, V1.S4]
	VLD1	(R2), [V20.S4, V21.S4]

loop2:
	MOVD	$·sha256KARM64(SB), R4
	VLD1.P	64(R1), [V4.B16, V5.B16, V6.B16, V7.B16]
	VLD1.P	64(R3), [V24.B16, V25.B16, V26.B16, V27.B16]
	VREV32	V4.B16, V4.B16
	VREV32	V24.B16, V24.B16
	VREV32	V5.B16, V5.B16
	VREV32	V25.B16, V25.B16
	VREV32	V6.B16, V6.B16
	VREV32	V26.B16, V26.B16
	VREV32	V7.B16, V7.B16
	VREV32	V27.B16, V27.B16
	VMOV	V0.B16, V2.B16
	VMOV	V1.B16, V3.B16
	VMOV	V20.B16, V22.B16
	VMOV	V21.B16, V23.B16

	SCHED2(V4, V5, V6, V7, V24, V25, V26, V27)
	SCHED2(V5, V6, V7, V4, V25, V26, V27, V24)
	SCHED2(V6, V7, V4, V5, V26, V27, V24, V25)
	SCHED2(V7, V4, V5, V6, V27, V24, V25, V26)
	SCHED2(V4, V5, V6, V7, V24, V25, V26, V27)
	SCHED2(V5, V6, V7, V4, V25, V26, V27, V24)
	SCHED2(V6, V7, V4, V5, V26, V27, V24, V25)
	SCHED2(V7, V4, V5, V6, V27, V24, V25, V26)
	SCHED2(V4, V5, V6, V7, V24, V25, V26, V27)
	SCHED2(V5, V6, V7, V4, V25, V26, V27, V24)
	SCHED2(V6, V7, V4, V5, V26, V27, V24, V25)
	SCHED2(V7, V4, V5, V6, V27, V24, V25, V26)
	FINAL2(V4, V24)
	FINAL2(V5, V25)
	FINAL2(V6, V26)
	FINAL2(V7, V27)

	VADD	V2.S4, V0.S4, V0.S4
	VADD	V3.S4, V1.S4, V1.S4
	VADD	V22.S4, V20.S4, V20.S4
	VADD	V23.S4, V21.S4, V21.S4
	SUB	$1, R5, R5
	CBNZ	R5, loop2

	VST1	[V0.S4, V1.S4], (R0)
	VST1	[V20.S4, V21.S4], (R2)
done2:
	RET

DATA	·sha256KARM64+0x00(SB)/4, $0x428a2f98
DATA	·sha256KARM64+0x04(SB)/4, $0x71374491
DATA	·sha256KARM64+0x08(SB)/4, $0xb5c0fbcf
DATA	·sha256KARM64+0x0c(SB)/4, $0xe9b5dba5
DATA	·sha256KARM64+0x10(SB)/4, $0x3956c25b
DATA	·sha256KARM64+0x14(SB)/4, $0x59f111f1
DATA	·sha256KARM64+0x18(SB)/4, $0x923f82a4
DATA	·sha256KARM64+0x1c(SB)/4, $0xab1c5ed5
DATA	·sha256KARM64+0x20(SB)/4, $0xd807aa98
DATA	·sha256KARM64+0x24(SB)/4, $0x12835b01
DATA	·sha256KARM64+0x28(SB)/4, $0x243185be
DATA	·sha256KARM64+0x2c(SB)/4, $0x550c7dc3
DATA	·sha256KARM64+0x30(SB)/4, $0x72be5d74
DATA	·sha256KARM64+0x34(SB)/4, $0x80deb1fe
DATA	·sha256KARM64+0x38(SB)/4, $0x9bdc06a7
DATA	·sha256KARM64+0x3c(SB)/4, $0xc19bf174
DATA	·sha256KARM64+0x40(SB)/4, $0xe49b69c1
DATA	·sha256KARM64+0x44(SB)/4, $0xefbe4786
DATA	·sha256KARM64+0x48(SB)/4, $0x0fc19dc6
DATA	·sha256KARM64+0x4c(SB)/4, $0x240ca1cc
DATA	·sha256KARM64+0x50(SB)/4, $0x2de92c6f
DATA	·sha256KARM64+0x54(SB)/4, $0x4a7484aa
DATA	·sha256KARM64+0x58(SB)/4, $0x5cb0a9dc
DATA	·sha256KARM64+0x5c(SB)/4, $0x76f988da
DATA	·sha256KARM64+0x60(SB)/4, $0x983e5152
DATA	·sha256KARM64+0x64(SB)/4, $0xa831c66d
DATA	·sha256KARM64+0x68(SB)/4, $0xb00327c8
DATA	·sha256KARM64+0x6c(SB)/4, $0xbf597fc7
DATA	·sha256KARM64+0x70(SB)/4, $0xc6e00bf3
DATA	·sha256KARM64+0x74(SB)/4, $0xd5a79147
DATA	·sha256KARM64+0x78(SB)/4, $0x06ca6351
DATA	·sha256KARM64+0x7c(SB)/4, $0x14292967
DATA	·sha256KARM64+0x80(SB)/4, $0x27b70a85
DATA	·sha256KARM64+0x84(SB)/4, $0x2e1b2138
DATA	·sha256KARM64+0x88(SB)/4, $0x4d2c6dfc
DATA	·sha256KARM64+0x8c(SB)/4, $0x53380d13
DATA	·sha256KARM64+0x90(SB)/4, $0x650a7354
DATA	·sha256KARM64+0x94(SB)/4, $0x766a0abb
DATA	·sha256KARM64+0x98(SB)/4, $0x81c2c92e
DATA	·sha256KARM64+0x9c(SB)/4, $0x92722c85
DATA	·sha256KARM64+0xa0(SB)/4, $0xa2bfe8a1
DATA	·sha256KARM64+0xa4(SB)/4, $0xa81a664b
DATA	·sha256KARM64+0xa8(SB)/4, $0xc24b8b70
DATA	·sha256KARM64+0xac(SB)/4, $0xc76c51a3
DATA	·sha256KARM64+0xb0(SB)/4, $0xd192e819
DATA	·sha256KARM64+0xb4(SB)/4, $0xd6990624
DATA	·sha256KARM64+0xb8(SB)/4, $0xf40e3585
DATA	·sha256KARM64+0xbc(SB)/4, $0x106aa070
DATA	·sha256KARM64+0xc0(SB)/4, $0x19a4c116
DATA	·sha256KARM64+0xc4(SB)/4, $0x1e376c08
DATA	·sha256KARM64+0xc8(SB)/4, $0x2748774c
DATA	·sha256KARM64+0xcc(SB)/4, $0x34b0bcb5
DATA	·sha256KARM64+0xd0(SB)/4, $0x391c0cb3
DATA	·sha256KARM64+0xd4(SB)/4, $0x4ed8aa4a
DATA	·sha256KARM64+0xd8(SB)/4, $0x5b9cca4f
DATA	·sha256KARM64+0xdc(SB)/4, $0x682e6ff3
DATA	·sha256KARM64+0xe0(SB)/4, $0x748f82ee
DATA	·sha256KARM64+0xe4(SB)/4, $0x78a5636f
DATA	·sha256KARM64+0xe8(SB)/4, $0x84c87814
DATA	·sha256KARM64+0xec(SB)/4, $0x8cc70208
DATA	·sha256KARM64+0xf0(SB)/4, $0x90befffa
DATA	·sha256KARM64+0xf4(SB)/4, $0xa4506ceb
DATA	·sha256KARM64+0xf8(SB)/4, $0xbef9a3f7
DATA	·sha256KARM64+0xfc(SB)/4, $0xc67178f2
GLOBL	·sha256KARM64(SB), RODATA|NOPTR, $256
