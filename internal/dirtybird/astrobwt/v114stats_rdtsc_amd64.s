//go:build v114stats && amd64

#include "textflag.h"

// func v114rdtsc() uint64
TEXT ·v114rdtsc(SB), NOSPLIT, $0-8
	RDTSC
	SHLQ $32, DX
	ORQ  DX, AX
	MOVQ AX, ret+0(FP)
	RET
