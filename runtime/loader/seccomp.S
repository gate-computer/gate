// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define SECCOMP_SET_MODE_FILTER 1

#define SECCOMP_RET_KILL_PROCESS 0x80000000
#define SECCOMP_RET_ALLOW        0x7fff0000

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

.Lseccomp_filters:
	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, 0				// Syscall number
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   SYS_recvfrom,      10,  0	// Third block
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   SYS_write,         16,  0	// Allow
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   SYS_mprotect,       2,  0	// Second block
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   SYS_rt_sigreturn,  14,  0	// Allow
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   SYS_exit_group,    13, 12	// Allow, kill

	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, (16 + 8 * 0 + 4)		// mprotect addr high half
	bpf_jump BPF_JMP | BPF_JGE | BPF_K, GATE_MIN_HEAP_HIGH32, 0, 12	// Kill
	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, (16 + 8 * 1 + 4)		// mprotect len high half
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   0,                  0, 10	// Kill
	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, (16 + 8 * 2)		// mprotect prot
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K, PROT_READ|PROT_WRITE, 7,  8	// Allow, kill

	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, (16 + 8 * 3)		// recvfrom flags
	bpf_stmt BPF_ALU | BPF_AND | BPF_K,   ~MSG_DONTWAIT		// Unsupported flags mask
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   0,                  0,  5	// Kill
	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, (16 + 8 * 5 + 0)		// recvfrom addrlen low half
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   0,                  0,  3	// Kill
	bpf_stmt BPF_LD  | BPF_W   | BPF_ABS, (16 + 8 * 5 + 4)		// recvfrom addrlen high half
	bpf_jump BPF_JMP | BPF_JEQ | BPF_K,   0,                  0,  1	// Kill

	bpf_stmt BPF_RET | BPF_K,             SECCOMP_RET_ALLOW
	bpf_stmt BPF_RET | BPF_K,             SECCOMP_RET_KILL_PROCESS
.Lseccomp_filters_end:
