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

#include "../poll.S"

#define INT3 0xcc

#define UCONTEXT_STACKLIMIT_REG_OFFSET 128 // rbx
#define UCONTEXT_STACKPTR_REG_OFFSET   160 // rsp
#define UCONTEXT_INSNPTR_REG_OFFSET    168 // rip

// Out: rcx = stack vars
// Clobbers rax.
.macro	stack_vars
	mov	$0xbffffffffffffffe, %rax
	mov	%rbx, %rcx
	and	%rax, %rcx			// Clear suspend bits
	sub	$GATE_STACK_LIMIT_OFFSET, %rcx
.endm

// Out: rcx = stack vars
//      out = current memory size in wasm pages
// Clobbers rax.
.macro	current_memory_pages out
	stack_vars
	mov	4(%rcx), \out
.endm

// Out: rsi = absolute buffer start address
//      rdx = buffer length
//      r11 = absolute buffer end address
//      r13 = current memory end address
//      r14 = memory address
// Clobbers rax.
.macro	io_prologue
	current_memory_pages %r13d
	shl	$16, %r13d			// Current memory size
	movq	-4*8(%r15), %r14		// Memory address
	add	%r14, %r13			// Current memory end address

	mov	16(%rsp), %esi			// Relative buffer address
	mov	8(%rsp), %edx			// Buffer length
	test	%edx, %edx
	je	rt_nop				// Nothing to transfer

	add	%r14, %rsi			// Absolute buffer address
	lea	(%rsi, %rdx), %r11		// End of absolute buffer

	cmp	%r11, %r13			// Check against memory limit
	jl	.Lout_of_bounds
	cmp	%rsi, %r11			// ...and detect wrap-around
	jl	.Lout_of_bounds
.endm

.section .text

.align	16,INT3
.globl	runtime_code_begin

runtime_code_begin:

	int3

.align	8,INT3

#include "../seccomp.S"

	int3

.align	16,INT3
.globl	runtime_init
.type	runtime_init,@function
.globl	runtime_init_no_sandbox
.type	runtime_init_no_sandbox,@function

// In:  rbx = stack limit
//      r15 = text address + init routine
runtime_init:

	// Unmap loader .text and .rodata sections

	mov	$GATE_LOADER_ADDR, %rdi		// munmap addr
	mov	$65536, %esi			// munmap length
	mov	$SYS_munmap, %eax
	syscall
	mov	$ERR_LOAD_MUNMAP_LOADER, %edi
	test	%eax, %eax
	jne	sys_exit

	// Build sock_fprog structure on stack

	lea	-16(%rsp), %rdx			// seccomp args
	mov	$seccomp_filter_len, %rax
	mov	%rax, 0(%rdx)
	lea	.Lseccomp_filter(%rip), %rax
	mov	%rax, 8(%rdx)

	// Enable seccomp

	mov	$SECCOMP_SET_MODE_FILTER, %edi	// seccomp mode
	xor	%esi, %esi			// seccomp flags
	mov	$SYS_seccomp, %eax
	syscall
	mov	$ERR_LOAD_SECCOMP, %edi
	test	%eax, %eax
	jne	sys_exit

	// Fall through

runtime_init_no_sandbox:

	// Terminate in uninitialized state if already suspended

	mov	$100+2, %edi			// Suspended
	test	$1, %ebx
	jne	sys_exit			// Don't use .Lexit

	// Mark stack as dirty just before execution.  (If SIGXCPU signal was
	// received just after the above check, the process has about a second
	// worth of CPU time to reach the first suspend check and execute
	// .Lexit to avoid inconsistent state.)

	stack_vars
	movl	$-1, (%rcx)			// stack_unused

	// Clear registers used by wag codegen

	xor	%eax, %eax			// Result when resuming
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
	xor	%r14d, %r14d

	// Execute wag object ABI init routine

	mov	%r15, %rcx			// Init routine address
	and	$~0x7f, %r15			// Text base
	jmp	retpoline

.align	16,INT3
.globl	signal_handler
.type	signal_handler,@function

// In:  edi = signum
//      rsi = siginfo
//      rdx = ucontext
signal_handler:
	cmp	$SIGSEGV, %edi
	je	.Lsigsegv

	mov	UCONTEXT_STACKLIMIT_REG_OFFSET(%rdx), %rbx
	stack_vars
	testl	$0x2, 20(%rcx)			// suspend_bits
	jne	.Ldo_not_modify_suspend_reg
	mov	$0x4000000000000001, %rax	// Suspend calls and loops
	or	%rax, UCONTEXT_STACKLIMIT_REG_OFFSET(%rdx)
.Ldo_not_modify_suspend_reg:
	orl	$0x1, 20(%rcx)			// suspend_bits
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
	mov	$100+5, %edi			// MemoryAccessOutOfBounds
	jmp	.Lexit

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

// In:  rax  = int result
//      xmm0 = float result
//      edx  = trap id
trap_handler:
	test	%edx, %edx
	je	.Ltrap_exit
	cmp	$4, %edx			// CallStackExhausted
	je	.Ltrap_call_stack_exhausted

	mov	%edx, %edi
	add	$100, %edi
	jmp	.Lexit

.Ltrap_exit:
	mov	%rax, %rdx

	stack_vars
	mov	%rdx, 32(%rcx)			// result[0]
	movq	%xmm0, 40(%rcx)			// result[1]

	mov	$1, %edi			// Status on failure
	test	%edx, %edx			// Program result
	cmove	%edx, %edi			// Status on success
	jmp	.Lexit

.Ltrap_call_stack_exhausted:
	test	$1, %ebx
	jne	.Ltrap_suspended

	mov	$100+4, %edi			// CallStackExhausted
	jmp	.Lexit

.Ltrap_suspended:
	mov	$100+2, %edi			// Suspended
	jmp	.Lexit

.align	16,INT3
.globl	current_memory
.type	current_memory,@function

// Out: eax = allocated wasm pages
current_memory:
	current_memory_pages %eax
	jmp	.Lresume

.align	16,INT3
.globl	grow_memory
.type	grow_memory,@function

// In:  eax = increment in wasm pages
// Out: eax = previously allocated wasm pages
grow_memory:
	mov	%eax, %r12d
	mov	%eax, %esi

	current_memory_pages %edi
	add	%rdi, %r12			// New memory pages
	cmp	%r12, -5*8(%r15)		// Memory grow limit in pages
	jl	.Loom

	movq	-4*8(%r15), %r14		// Memory address

#ifdef __ANDROID__

	xor	%r10d, %r10d			// mremap flags
	mov	%r12d, %edx
	shl	$16, %rdx			// mremap new_size
	mov	%edi, %esi
	shl	$16, %rsi			// mremap old_size
	mov	%r14, %rdi			// mremap old_addr

	mov	$SYS_mremap, %eax
	syscall
	test	$0xfff, %eax
	je	.Lgrow_memory_done

	mov	$ERR_RT_MREMAP, %eax
	jmp	.Lexit

#else // !__ANDROID__

	shl	$16, %rdi			// Current memory bytes
	add	%r14, %rdi			// mprotect addr

	shl	$16, %rsi			// mprotect len
	je	.Lgrow_memory_done

	mov	$PROT_READ|PROT_WRITE, %edx
	mov	$SYS_mprotect, %eax
	syscall
	test	%eax, %eax
	je	.Lgrow_memory_done

	mov	$ERR_RT_MPROTECT, %eax
	jmp	.Lexit

#endif // __ANDROID__

.Lgrow_memory_done:
	current_memory_pages %eax
	mov	%r12d, 4(%rcx)			// current_memory_pages
	jmp	.Lresume

.Loom:
	mov	$-1, %eax
	jmp	.Lresume

.align	16,INT3
.globl	rt_nop
.type	rt_nop,@function

rt_nop:
	xor	%eax, %eax

	// Fall through

.Lresume:
	lea	0x20(%r15), %rcx		// wag object ABI resume routine

	// Fall through

.globl	retpoline
.type	retpoline,@function

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

.align	16,INT3
.globl	rt_poll
.type	rt_poll,@function

rt_poll:
	mov	16(%rsp), %ecx			// Input events
	mov	8(%rsp), %esi			// Output events

	sub	$8*2, %rsp			// sizeof(struct pollfd) * 2
	movl	$GATE_INPUT_FD, (%rsp)		// fds[0].fd
	mov	%ecx, 4(%rsp)			// fds[0].events and .revents
	movl	$GATE_OUTPUT_FD, 8(%rsp)	// fds[1].fd
	mov	%esi, 12(%rsp)			// fds[1].events and .revents

	mov	%rsp, %rdi			// fds
	mov	$2, %esi			// nfds
	xor	%edx, %edx			// tmo_p
	xor	%r10d, %r10d			// sigmask
	mov	$SYS_ppoll, %eax
	syscall

	xor	%ecx, %ecx
	xor	%esi, %esi
	mov	6(%rsp), %cx			// Input revents
	mov	14(%rsp), %si			// Output revents
	add	$8*2, %rsp			// sizeof(struct pollfd) * 2

	mov	$ERR_RT_POLL, %edi

	test	%rax, %rax
	jg	.Lpoll_revents

	cmp	$-EAGAIN, %eax
	je	rt_nop
	cmp	$-EINTR, %eax
	je	rt_nop

	jmp	.Lexit

.Lpoll_revents:
	xor	%eax, %eax

	test	$POLLHUP|POLLRDHUP, %ecx	// Being suspended?
	jne	rt_nop
	test	$~POLLIN, %ecx
	jne	.Lexit
	or	%ecx, %eax

	test	$~POLLOUT, %esi
	jne	.Lexit
	or	%esi, %eax

	jmp	.Lresume

.align	16,INT3
.globl	rt_read
.type	rt_read,@function

rt_read:
	io_prologue

	mov	$GATE_INPUT_FD, %edi
	mov	$SYS_read, %eax
	syscall
	test	%rax, %rax
	jge	.Lresume			// EOF if being suspended or killed

	cmp	$-EAGAIN, %eax
	je	rt_nop
	cmp	$-EINTR, %eax
	je	rt_nop

	mov	$ERR_RT_READ, %edi
	jmp	.Lexit

.align	16,INT3
.globl	rt_write
.type	rt_write,@function

rt_write:
	io_prologue

	mov	$GATE_OUTPUT_FD, %edi
	mov	$SYS_write, %eax
	syscall
	test	%rax, %rax
	jg	.Lresume

	cmp	$-EAGAIN, %eax
	je	rt_nop
	cmp	$-EINTR, %eax
	je	rt_nop

	mov	$ERR_RT_WRITE, %edi
	jmp	.Lexit

.Lout_of_bounds:
	mov	$100+5, %edi			// MemoryAccessOutOfBounds
	jmp	.Lexit

.align	16,INT3
.globl	rt_time
.type	rt_time,@function

rt_time:
	mov	8(%rsp), %edi			// clk_id

	// Fall through

// In:  edi = clock id
.Ltime:
	mov	%edi, %r12d

	sub	$16, %rsp			// sizeof(struct timespec)
	mov	%rsp, %rsi			// tp

	stack_vars
	orl	$0x2, 20(%rcx)			// suspend_bits

	push	%rbx
	push	%r15

	mov	-11*8(%r15), %rcx		// clock_gettime C ABI function
	call	retpoline
	mov	%eax, %r13d

	pop	%r15
	pop	%rbx

	stack_vars
	xor	%eax, %eax
	xchg	%eax, 20(%rcx)			// suspend_bits
	test	$0x1, %eax
	je	.Lnot_suspended
	mov	$0x4000000000000001, %rax	// Suspend calls and loops
	or	%rax, %rbx			//

.Lnot_suspended:
	pop	%r10				// tv_sec
	pop	%r11				// tv_nsec

	mov	$ERR_RT_CLOCK_GETTIME, %edi
	mov	$-1, %rdx			// Outrageous timestamp
	test	%r13d, %r13d
	jne	.Lexit_time

	mov	%r10, %rax
	mov	$1000000000, %ecx
	mul	%rcx
	and	-9*8(%r15), %r11d		// mask
	add	%r11, %rax			// (tv_sec * 1000000000) + (tv_nsec & mask)

	cmp	$6, %r12d			// CLOCK_MONOTONIC_COARSE
	jne	.Lresume

	sub	-10*8(%r15), %rax		// local_monotonic_time_base
	mov	%rax, %rdx
	stack_vars
	add	8(%rcx), %rdx			// monotonic_time_snapshot
	mov	%rdx, %rax
	jmp	.Lresume

.align	16,INT3
.globl	rt_random
.type	rt_random,@function

rt_random:
	stack_vars
	mov	16(%rcx), %edx			// random_avail
	dec	%edx
	jl	.Lno_random
	mov	%edx, 16(%rcx)			// random_avail

	xor	%eax, %eax
	mov	-8*8(%r15, %rdx), %al		// random
	jmp	.Lresume

.Lno_random:
	mov	$-1, %eax
	jmp	.Lresume

.align	16,INT3
.globl	rt_trap
.type	rt_trap,@function

rt_trap:
	mov	8(%rsp), %edi
	mov	%rbp, %rsp			// Restart caller on resume

	// Fall through

// In:  edi = status code
.Lexit:
	pushq	%rdi

	mov	$6, %edi			// CLOCK_MONOTONIC_COARSE
	call	.Ltime
	mov	%rax, %rdx

	popq	%rdi

	// Fall through

// In:  edi = status code
//	rdx = monotonic time
.Lexit_time:
	stack_vars
	mov	%rsp, %rax
	sub	%rcx, %rax
	mov	%eax, (%rcx)			// stack_unused
	mov	%rdx, 8(%rcx)			// monotonic_time_snapshot

	// Fall through

.globl	sys_exit
.type	sys_exit,@function

// In:  edi = status code
sys_exit:
	mov	$SYS_exit_group, %eax
	syscall
	int3

.align	16,INT3
.globl	rt_debug
.type	rt_debug,@function

rt_debug:
	io_prologue

	mov	$SYS_write, %eax
	mov	$GATE_DEBUG_FD, %edi
	syscall
	test	%rax, %rax
	jg	.Ldebugged_some
	je	rt_nop				// EOF; being suspended or killed

	cmp	$-EINTR, %eax
	je	rt_debug

	mov	$ERR_RT_DEBUG, %edi
	jmp	.Lexit

.Ldebugged_some:
	add	%eax, 16(%rsp)
	sub	%eax, 8(%rsp)
	je	rt_nop

	jmp	rt_debug

	int3

.align	16,INT3
.globl	runtime_code_end

runtime_code_end:

.section .note.GNU-stack,"",%progbits		// Don't want executable stack
