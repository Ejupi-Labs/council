//go:build darwin && (amd64 || arm64)

#include "textflag.h"

TEXT council_apply_fgetattrlist_trampoline<>(SB), NOSPLIT, $0-0
	JMP council_apply_fgetattrlist(SB)

GLOBL ·applyDarwinFgetattrlistAddr(SB), RODATA, $8
DATA ·applyDarwinFgetattrlistAddr(SB)/8, $council_apply_fgetattrlist_trampoline<>(SB)
