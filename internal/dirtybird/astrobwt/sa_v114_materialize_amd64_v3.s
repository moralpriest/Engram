//go:build amd64.v3

#include "textflag.h"

TEXT ·materializeOrigins(SB), NOSPLIT, $0-24
	MOVQ dst+0(FP), AX
	MOVQ src+8(FP), CX
	MOVL count+16(FP), DX
	VPBROADCASTD rel+20(FP), Y0
loop:
	VMOVDQU (CX), Y1
	VPADDD Y0, Y1, Y1
	VMOVDQU Y1, (AX)
	ADDQ $32, AX
	ADDQ $32, CX
	SUBL $8, DX
	JG loop
	VZEROUPPER
	RET

// Consume consecutive unique literal or short-arena runs. The caller supplies
// padded SA/arena buffers and handles collisions and long copies in Go.
TEXT ·writeUniqueRunBatch(SB), NOSPLIT, $0-80
	MOVQ sa+0(FP), DI
	MOVQ runs+8(FP), SI
	MOVQ arena+16(FP), DX
	MOVQ n+24(FP), R8
	MOVQ groupStart+32(FP), AX
	MOVQ outPos+40(FP), BX
	MOVQ logicalLen+48(FP), R12
	MOVQ arenaCap+56(FP), R13
unique_loop:
	CMPQ AX, R8
	JGE unique_done
	LEAQ (SI)(AX*8), R9
	MOVL 0(R9), R10
	LEAQ 1(AX), R11
	CMPQ R11, R8
	JGE unique_copy
	CMPL 8(R9), R10
	JE unique_done
unique_copy:
	MOVL 4(R9), R10
	MOVL R10, R11
	SHRL $17, R11
	JNZ unique_arena
	CMPQ BX, R12
	JGE unique_done
	MOVL R10, (DI)(BX*4)
	INCQ BX
	INCQ AX
	JMP unique_loop
unique_arena:
	CMPL R11, $8
	JG unique_done
	LEAQ (BX)(R11*1), R14
	CMPQ R14, R12
	JG unique_done
	ANDL $0x1ffff, R10
	LEAQ 8(R10), R14
	CMPQ R14, R13
	JG unique_done
	VMOVDQU (DX)(R10*4), Y0
	VMOVDQU Y0, (DI)(BX*4)
	ADDQ R11, BX
	INCQ AX
	JMP unique_loop
unique_done:
	MOVQ AX, ret0+64(FP)
	MOVQ BX, ret1+72(FP)
	VZEROUPPER
	RET
