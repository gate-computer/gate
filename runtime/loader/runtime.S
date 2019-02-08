// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define __ASSEMBLY__

#include <asm/errno.h>
#include <asm/mman.h>
#include <asm/signal.h>
#include <linux/bpf_common.h>
#include <sys/syscall.h>

#include "errors.h"
#include "runtime.h"

#define INT3 0xcc

#define MSG_DONTWAIT_BIT 6
#define MSG_DONTWAIT (1 << MSG_DONTWAIT_BIT)

#define SECCOMP_SET_MODE_FILTER 1

#define SECCOMP_RET_KILL_PROCESS 0x80000000
#define SECCOMP_RET_ALLOW        0x7fff0000

#define UCONTEXT_SUSPEND_REG_OFFSET  128 // rbx
#define UCONTEXT_STACKPTR_REG_OFFSET 160 // rsp
#define UCONTEXT_INSNPTR_REG_OFFSET  168 // rip

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

.section .text

.align	16,INT3
.globl	runtime_code_begin

runtime_code_begin:

	int3

.align	8,INT3

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

	int3

.align	16,INT3
.globl	runtime_init
.type	runtime_init,@function
.globl	runtime_init_no_sandbox
.type	runtime_init_no_sandbox,@function

// In:  r12 = text address + init routine
//      r13 = stack limit
//      r14 = memory address
runtime_init:
	// Unmap loader .text and .rodata sections.

	mov	$GATE_LOADER_ADDR, %rdi		// munmap addr
	mov	$GATE_LOADER_SIZE, %esi		// munmap length
	mov	$SYS_munmap, %eax
	syscall
	mov	$ERR_LOAD_MUNMAP_LOADER, %edi
	test	%rax, %rax
	jne	sys_exit

	// Build sock_fprog structure on stack.  Using 16 bytes of red zone.

	lea	-16(%rsp), %rdx			// seccomp args
	mov	$(.Lseccomp_filters_end - .Lseccomp_filters) / 8, %rax
	mov	%rax, 0(%rdx)
	lea	.Lseccomp_filters(%rip), %rax
	mov	%rax, 8(%rdx)

	// Enable seccomp.

	mov	$SECCOMP_SET_MODE_FILTER, %edi	// seccomp mode
	xor	%esi, %esi			// seccomp flags
	mov	$SYS_seccomp, %eax
	syscall
	mov	$ERR_LOAD_SECCOMP, %edi
	test	%rax, %rax
	jne	sys_exit

	// Fall through to runtime_init_no_sandbox.

runtime_init_no_sandbox:
	// Initialize reserved registers.

	mov	%r13, %rbx			// Stack limit
	mov	%r12, %r15
	and	$~0x7f, %r15			// Text base
	mov	%r12, %rcx			// Init routine address

	// Clear unused registers.

	xor	%eax, %eax
	xor	%edx, %edx
	xor	%ebp, %ebp
	xor	%esi, %esi
	xor	%edi, %edi
	xor	%r8d, %r8d
	xor	%r9d, %r9d
	xor	%r10d, %r10d
	xor	%r11d, %r11d
	xor	%r12d, %r12d
	xor	%r13d, %r13d

	// Execute wag object ABI init routine using retpoline.

	jmp	retpoline

.align	16,INT3
.globl	signal_handler
.type	signal_handler,@function

// In:  edi = signum
//      rsi = siginfo
//      rdx = ucontext
signal_handler:
	cmp	$SIGIO, %edi
	je	.Lsigio

	cmp	$SIGSEGV, %edi
	je	.Lsigsegv

	mov	$0x8000000000000001, %rax	// Suspend calls and loops.
	or	%rax, UCONTEXT_SUSPEND_REG_OFFSET(%rdx)

.Lsigio:
	ret

.Lsigsegv:
	mov	UCONTEXT_STACKPTR_REG_OFFSET(%rdx), %rax
	sub	$8, %rax
	mov	%rax, UCONTEXT_STACKPTR_REG_OFFSET(%rdx)

	mov	UCONTEXT_INSNPTR_REG_OFFSET(%rdx), %rbx
	mov	%rbx, (%rax)

	lea	.Lsigsegv_exit(%rip), %rbx
	mov	%rbx, UCONTEXT_INSNPTR_REG_OFFSET(%rdx)
	ret

.Lsigsegv_exit:
	mov	$105, %edi			// MemoryAccessOutOfBounds
	jmp	.Lruntime_exit

.align	16,INT3
.globl	signal_restorer
.type	signal_restorer,@function

signal_restorer:
	mov	$SYS_rt_sigreturn, %eax
	syscall
	int3

.align	16,INT3
.globl	trap_handler
.type	trap_handler,@function

// In:  eax = trap id
trap_handler:
	test	%eax, %eax
	je	.Ltrap_exit

	cmp	$2, %eax			// Suspended
	je	.Lsuspended

	cmp	$4, %eax			// CallStackExhausted
	je	.Lcall_stack_exhausted

	mov	%eax, %edi
	add	$100, %edi			// exit status
	jmp	.Lruntime_exit

.Ltrap_exit:
	mov	$1, %edi			// exit status on failure
	shrq	$32, %rax			// Program result
	cmove	%eax, %edi			// exit status on success
	jmp	.Lruntime_exit

.Lsuspended:
	mov	$100+2, %edi			// exit status: Suspended
	jmp	.Lruntime_exit

.Lcall_stack_exhausted:
	test	$1, %ebx
	jne	.Lsuspended

	mov	$100+4, %edi			// exit status: CallStackExhausted
	jmp	.Lruntime_exit

.align	16,INT3
.globl	current_memory
.type	current_memory,@function

// Out: eax = allocated wasm pages
current_memory:
	mov	-GATE_STACK_LIMIT_OFFSET(%rbx), %eax
	jmp	.Lresume

.align	16,INT3
.globl	grow_memory
.type	grow_memory,@function

// In:  eax = increment in wasm pages
// Out: eax = previously allocated wasm pages
grow_memory:
	mov	%eax, %r12d

	mov	-GATE_STACK_LIMIT_OFFSET(%rbx), %edi
	add	%rdi, %r12			// New memory pages
	cmp	%r12, -32(%r15)			// Grow memory limit pages
	jl	.Loom

	shl	$16, %rdi			// Current memory bytes
	add	%r14, %rdi			// mprotect addr

	mov	%eax, %esi
	shl	$16, %rsi			// mprotect len
	je	.Lgrow_done

	mov	$PROT_READ|PROT_WRITE, %edx
	mov	$SYS_mprotect, %eax
	syscall
	test	%rax, %rax
	je	.Lgrow_done

	mov	$ERR_RT_MPROTECT, %eax
	jmp	trap_handler

.Lgrow_done:
	mov	-GATE_STACK_LIMIT_OFFSET(%rbx), %eax
	mov	%r12d, -GATE_STACK_LIMIT_OFFSET(%rbx)
	jmp	.Lresume

.Loom:
	mov	$-1, %eax
	jmp	.Lresume

.align	16,INT3
.globl	retpoline
.type	retpoline,@function

.Lresume:
	lea	16(%r15), %rcx			// wag object ABI resume routine

	// Fall through to retpoline.

// In:  rcx = target address
retpoline:
	call	.Lretpoline_setup
.Lretpoline_capture:
	pause
	jmp	.Lretpoline_capture
.Lretpoline_setup:
	mov	%rcx, (%rsp)
	xor	%ecx, %ecx
	ret

// In:  r12 = relative length address (zero-extended 32-bit value)
//      r13 = current memory end address
// Out: r12 = absolute length address
//      r13 = current memory end address
.macro	io_len_check nop_label
	test	%r12d, %r12d
	je	\nop_label

	add	%r14, %r12			// Absolute length address.
	lea	4(%r12), %r11			// End of absolute length.

	cmp	%r11, %r13			// Check against memory limit,
	jl	.Lout_of_bounds			//
	cmp	%r12, %r11			// and detect wrap-around.
	jl	.Lout_of_bounds			//
.endm

// In:  rsi = relative buffer address (zero-extended 32-bit value)
//      r12 = absolute length address
//      r13 = current memory end address
// Out: rdx = length
//      rsi = absolute buffer address
//      r12 = absolute length address
//      r13 = current memory end address
.macro	io_setup nop_label
	mov	(%r12), %edx			// Length (zero-extended).
	test	%edx, %edx			//
	je	\nop_label			// Nothing to transfer.

	add	%r14, %rsi			// Absolute buffer address.
	lea	(%rsi, %rdx), %r11		// End of absolute buffer.

	cmp	%r11, %r13			// Check against memory limit,
	jl	.Lout_of_bounds			//
	cmp	%rsi, %r11			// and detect wrap-around.
	jl	.Lout_of_bounds			//
.endm

.align	16,INT3
.globl	gate_io
.type	gate_io,@function

gate_io:
	test	$1, %ebx
	jne	.Lsuspended

	mov	-GATE_STACK_LIMIT_OFFSET(%rbx), %r13d
	shl	$16, %r13d			// Current memory size
	add	%r14, %r13			// Current memory end address

	// Setup source buffer.

	mov	24(%rsp), %esi			// Relative buffer address
	mov	16(%rsp), %r12d			// Relative length address
	io_len_check .Lskip_send
	io_setup .Lskip_send

	// Send.

	mov	$SYS_write, %eax
	mov	$GATE_OUTPUT_FD, %edi
	syscall

	// Handle normal write result.

	test	%rax, %rax
	jg	.Lsent

	// Handle write errors.

	test	$1, %ebx
	jne	.Lsuspended

	cmp	$-EAGAIN, %eax
	je	.Lnot_sent
	cmp	$-EINTR, %eax
	je	.Lnot_sent

	mov	$ERR_RT_SEND, %edi		// exit status
	jmp	.Lruntime_exit

.Lnot_sent:
	xor	%eax, %eax			// Sent 0 bytes

.Lsent:
	mov	%eax, (%r12)			// Sent length

.Lskip_send:
	// Setup target buffer.

	mov	40(%rsp), %esi			// Relative buffer address
	mov	32(%rsp), %r12d			// Relative length address
	io_len_check .Lskip_receive
	io_setup .Lskip_receive

	// Map gate_io flag to recv flag.

	mov	8(%rsp), %r10d			// I/O flags
	and	$1, %r10d			// Discard unknown flags
	xor	$1, %r10d			// Invert IO_RECV_WAIT flag
	shl	$MSG_DONTWAIT_BIT, %r10d

	// No source address/length.

	xor	%r8d, %r8d
	xor	%r9d, %r9d

	// Receive.

	mov	$SYS_recvfrom, %eax
	xor	%edi, %edi			// GATE_INPUT_FD
	syscall

	// Handle normal read result.

	test	%rax, %rax
	jg	.Lreceived
	je	.Lreceived_eof

	// Handle read errors.

	test	$1, %ebx
	jne	.Lsuspended

	cmp	$-EAGAIN, %eax
	je	.Lnot_received
	cmp	$-EINTR, %eax
	je	.Lnot_received

	mov	$ERR_RT_RECV, %edi		// exit status
	jmp	.Lruntime_exit

.Lnot_received:
	xor	%eax, %eax			// Received 0 bytes

.Lreceived:
	mov	%eax, (%r12)			// Received length

.Lskip_receive:
	xor	%eax, %eax
	jmp	.Lresume

.Lreceived_eof:
	mov	%eax, (%r12)			// Received length
	jmp	.Lsuspended

.Lout_of_bounds:
	mov	$5, %eax			// MemoryAccessOutOfBounds trap
	jmp	trap_handler

.align	16,INT3
.globl	gate_nop
.type	gate_nop,@function

gate_nop:
	test	$1, %ebx
	jne	.Lsuspended

	xor	%eax, %eax
	jmp	.Lresume

.align	16,INT3
.globl	gate_exit
.type	gate_exit,@function

gate_exit:
	mov	8(%rsp), %ecx			// gate_exit status number

	xor	%edi, %edi
	test	%ecx, %ecx
	setne	%dil				// exit syscall status bit

	// Fall through to .Lruntime_exit.

// In:  edi = status code
.Lruntime_exit:
	mov	$0x7ffffffffffffffe, %rax	// Clear suspend bits.
	and	%rax, %rbx
	sub	$GATE_STACK_LIMIT_OFFSET, %rbx	// Stack buffer.
	mov	%rsp, %rax
	sub	%rbx, %rax			// Stack offset.

	// Write variables at start of stack space.
	// (Current memory size is already there.)

	mov	%eax, 4(%rbx)			// Stack offset.
	mov	%r15, 8(%rbx)			// Text address.

	// Fall through to sys_exit.

.globl	sys_exit
.type	sys_exit,@function

// In:  edi = status code
sys_exit:
	mov	$SYS_exit_group, %eax
	syscall
	int3

.align	16,INT3
.globl	gate_debug
.type	gate_debug,@function

gate_debug:
	test	$1, %ebx
	jne	.Lsuspended

	mov	-GATE_STACK_LIMIT_OFFSET(%rbx), %r13d
	shl	$16, %r13d			// Current memory size
	add	%r14, %r13			// Current memory end address

	mov	16(%rsp), %esi			// Relative buffer address
	lea	8(%rsp), %r12			// Absolute length address
	io_setup .Lskip_debug

	mov	$SYS_write, %eax
	mov	$GATE_DEBUG_FD, %edi
	syscall

	test	%rax, %rax
	jge	.Ldebugged

	cmp	$-EINTR, %eax
	je	gate_debug

	mov	$ERR_RT_DEBUG, %edi		// exit status
	jmp	.Lruntime_exit

.Ldebugged:
	add	%eax, 16(%rsp)
	sub	%eax, 8(%rsp)
	jne	gate_debug

.Lskip_debug:
	xor	%eax, %eax
	jmp	.Lresume

	int3

.align	16,INT3
.globl	runtime_code_end

runtime_code_end:

// Don't want executable stack.
.section .note.GNU-stack,"",%progbits
