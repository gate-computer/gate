// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <asm/mman.h>
#include <linux/bpf_common.h>
#include <sys/syscall.h>

#define SECCOMP_RET_KILL_PROCESS 0x80000000
#define SECCOMP_RET_ALLOW        0x7fff0000

#ifdef __ANDROID__
#define GROW_SYSCALL SYS_mremap
#else
#define GROW_SYSCALL SYS_mprotect
#endif

.macro	bpf_stmt code, k
.short	\code
.byte	0
.byte	0
.int	\k
.endm

.macro	bpf_jump code, k, jt, jf
.short	\code
.byte	\jt
.byte	\jf
.int	\k
.endm

.Lseccomp_filter:
	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, 0				// Syscall number
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   SYS_read,          12, 0	// Allow
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   SYS_write,         11, 0	// Allow
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   SYS_ppoll,         10, 0	// Allow
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   GROW_SYSCALL,       3, 0	// Second block
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   SYS_rt_sigreturn,   8, 0	// Allow
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   SYS_close,          7, 0	// Allow
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   SYS_exit_group,     6, 5	// Allow, kill

#ifdef __ANDROID__
	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, (16 + 8 * 1 + 4)		// mremap old_size high half
	bpf_jump BPF_JMP | BPF_JGE | BPF_K,   0,                  0, 5	// Kill
	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, (16 + 8 * 2 + 4)		// mremap new_size high half
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   0,                  0, 3	// Kill
	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, (16 + 8 * 3)		// mremap flags
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   0,                  0, 1	// Allow, kill
#else
	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, (16 + 8 * 0 + 4)		// mprotect addr high half
	bpf_jump BPF_JMP | BPF_JGE | BPF_K, GATE_MIN_HEAP_HIGH32, 0, 5	// Kill
	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, (16 + 8 * 1 + 4)		// mprotect len high half
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   0,                  0, 3	// Kill
	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, (16 + 8 * 2)		// mprotect prot
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K, PROT_READ|PROT_WRITE, 0, 1	// Allow, kill
#endif

	bpf_stmt BPF_RET | BPF_K,             SECCOMP_RET_ALLOW
	bpf_stmt BPF_RET | BPF_K,             SECCOMP_RET_KILL_PROCESS

.equ	.Lseccomp_filter_len, (. - .Lseccomp_filter) / 8
