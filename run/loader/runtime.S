// Copyright (c) 2016 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <asm/errno.h>
#include <sys/syscall.h>

#include "../defs.h"

#define INT3 0xcc

	.section .runtime,"ax",%progbits

	.align	16,INT3

	.rept	16
	int3
	.endr

	.globl	runtime_start_with_syscall
	.type	runtime_start_with_syscall,@function

// rax = munmap syscall number
// rdi = loader .text section address (1st syscall arg)
// rsi = loader .text section size (2nd syscall arg)
// r9  = suspend flag
// r12 = text address
// r13 = stack limit
// r14 = memory address
// r15 = current memory limit
// mm0 = trap handler
// mm1 = memory growth limit
runtime_start_with_syscall:
	// unmap loader .text section (parameters supplied by the caller)
	syscall
	mov     $80, %edi
	test    %rax, %rax
	jne     .Lexit

	// enable seccomp
	xor	%edi, %edi		// mode (SECCOMP_SET_MODE_STRICT)
	xor	%esi, %esi		// flags
	xor	%edx, %edx		// args
	mov	$SYS_seccomp, %eax
	syscall
	mov	$81, %edi
	test	%rax, %rax
	jne	.Lexit

	// clear unused registers
	xor	%ecx, %ecx
	xor	%ebx, %ebx
	xor	%ebp, %ebp
	xor	%esi, %esi
	xor	%edi, %edi
	xor	%r8d, %r8d
	xor	%r10d, %r10d
	xor	%r11d, %r11d

	// 0 = no resume
	xor	%eax, %eax

	// skip trap code
	mov	%r12, %rdx
	add	$16, %rdx
	jmp	*%rdx

	.align	16,INT3

	.rept	16
	int3
	.endr

	.globl	signal_handler
	.type	signal_handler,@function

// edi = signum
signal_handler:
	movq	$1, GATE_SIGNAL_STACK_R9_OFFSET(%rsp)
	ret

	.align	16,INT3

	.rept	16
	int3
	.endr

	.globl	signal_restorer
	.type	signal_restorer,@function

signal_restorer:
	mov	$SYS_rt_sigreturn, %eax
	syscall

	.align	16,INT3

	.rept	16
	int3
	.endr

	.globl	trap_handler
	.type	trap_handler,@function

// eax = trap id
trap_handler:
	test	%eax, %eax
	je	.Ltrap_exit

	cmp	$1, %eax		// missing function
	je	.Ltrap_io

	cmp	$2, %eax		// suspended
	je	.Ltrap_io

	mov	%eax, %edi		// zero-extend
	add	$100, %edi		// status (1st syscall arg)
	jmp	.Lexit

.Ltrap_exit:
	mov	$1, %edi		// failure status (1st syscall arg)
	shrq	$32, %rax		// exit code
	cmove	%eax, %edi		// success status (1st syscall arg)
	jmp	.Lexit

.Ltrap_io:
	// using 40 bytes of the 128-byte stack red zone

	// save registers possibly used as parameters of the missing function
	mov	%rcx, -8(%rsp)		// clobbered by syscall
	mov	%rsi, -16(%rsp)		// used for syscall arg
	mov	%rdi, -24(%rsp)		// used for syscall arg
	mov	%r11, -32(%rsp)		// clobbered by syscall

	mov	$GATE_OUTPUT_FD, %edi	// fd (1st syscall arg)
	lea	-40(%rsp), %rsi		// ptr (2nd syscall arg)
	mov	$8, %edx		// size (3rd syscall arg)

	mov	%edx, (%rsi)		// packet size
	movw	$GATE_PACKET_FLAG_TRAP, 4(%rsi)
	mov	%ax, 6(%rsi)		// packet code (trap)

.Lloop_mf:
	mov	$SYS_write, %eax
	syscall
	test	%rax, %rax
	jle	.Lerror

	add	%rax, %rsi		// update ptr (2nd syscall arg)
	sub	%eax, %edx		// update size (3rd syscall arg)
	jne	.Lloop_mf

	mov	$GATE_WAKEUP_FD, %edi	// fd (1st syscall arg)
	lea	-40(%rsp), %rsi		// ptr (2nd syscall arg)
	mov	$1, %edx		// size (3rd syscall arg)
	mov	$SYS_read, %eax
	syscall
	test	%rax, %rax
	jl	.Lerror			// EOF is ok

	mov	-32(%rsp), %r11
	mov	-24(%rsp), %rdi
	mov	-16(%rsp), %rsi
	mov	-8(%rsp), %rcx

	// rewind return address before the call instruction that got us here
	subq	$5, (%rsp)
	ret

	.align	128,INT3 // important ABI functions fit in 128 bytes; align naturally

	.globl	__gate_send
	.type	__gate_send,@function

// ecx = relative addr
// ebx = size
__gate_send:
	mov	$SYS_write, %ebp
	mov	$GATE_OUTPUT_FD, %edi	// fd (1st syscall arg)
	// TODO: check that no internal flags were specified
	jmp	.Lio

	.align	16,INT3

	.globl	__gate_recv
	.type	__gate_recv,@function

// ecx = relative addr
// ebx = size
// ebp = flags
__gate_recv:
	mov	$GATE_NONBLOCK_FD, %eax
	xor	%edi, %edi		// GATE_BLOCK_FD (1st syscall arg)
	shr	$1, %ebp		// flag parameter to CPU carry flag
	cmovc	%eax, %edi		// fd (1st syscall arg)
	xor	%ebp, %ebp		// SYS_read
	// fall through to .Lio

// ecx = relative addr
// ebx = size
// rdi = fd (1st syscall arg)
// rbp = syscall
// result: remaining size
.Lio:
	// check lower bound of relative addr
	and	%ecx, %ecx		// zero-extend relative addr + test
	js	.Lbounds

	lea	(%r14,%rcx), %rsi	// absolute addr (2nd syscall arg)

	// check lower bound of size
	mov	%ebx, %edx		// zero-extend size (3rd syscall arg)
	test	%edx, %edx
	je	.Lempty
	js	.Lbounds

	// check upper bound of absolute addr + size
	lea	(%rsi,%rdx), %r11
	cmp	%r11, %r15		// against current memory limit
	jl	.Lbounds
	cmp	%rsi, %r11		// against absolute addr (wrap-around)
	jl	.Lbounds
.Lloop_io:
	mov	%ebp, %eax		// syscall
	syscall
	test	%rax, %rax
	jle	.Lerror

	add	%rax, %rsi		// update absolute addr (2nd syscall arg)
	sub	%eax, %edx		// update size (3rd syscall arg)
	jne	.Lloop_io
.Lempty:
	xor	%eax, %eax		// remaining size (result)
	ret

.Lerror:
	mov	$90, %edi		// status (1st syscall arg)
	cmp	$-EAGAIN, %rax
	jne	.Lexit
	mov	%edx, %eax		// remaining size (result)
	ret

.Lbounds:
	mov	$91, %edi		// status (1st syscall arg)
	jmp	.Lexit

	.align	16,INT3

	.globl	__gate_exit
	.type	__gate_exit,@function

// ecx = status number
__gate_exit:
	xor	%edi, %edi
	test	%ecx, %ecx
	setne	%dil			// status boolean (1st syscall arg)
	// fall through to .Lexit

	.globl	runtime_exit
	.type	runtime_exit,@function

// edi = status code
runtime_exit:
.Lexit:
	sub	$8+128, %r13		// stack reserve
	mov	%rsp, %rax
	sub	%r13, %rax
	mov	%rax, (%r13)		// first word of stack space

	mov	$SYS_exit, %eax
	syscall
	int3

	.align	16,INT3

	.globl	__gate_func_ptr
	.type	__gate_func_ptr,@function

__gate_func_ptr:
	xor	%eax, %eax		// null ptr (result)
	ret

	.align	16,INT3

	.globl	__gate_get_abi_version
	.type	__gate_get_abi_version,@function

__gate_get_abi_version:
	mov	$GATE_ABI_VERSION, %eax
	ret

	.align	16,INT3

	.globl	__gate_get_arg
	.type	__gate_get_arg,@function

__gate_get_arg:
	movq	%mm6, %rax
	ret

	.align	16,INT3

	.globl	__gate_get_max_packet_size
	.type	__gate_get_max_packet_size,@function

__gate_get_max_packet_size:
	mov	$GATE_MAX_PACKET_SIZE, %eax
	ret

	.align	16,INT3

	.globl	__gate_debug_write
	.type	__gate_debug_write,@function

// ecx = relative addr
// ebx = size
__gate_debug_write:
	mov	$SYS_write, %ebp
	mov	$GATE_DEBUG_FD, %edi	// fd (1st syscall arg)
	jmp	.Lio
