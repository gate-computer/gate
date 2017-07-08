#include <asm/errno.h>
#include <sys/syscall.h>

#include "../defs.h"

.section .runtime,"ax",%progbits

	.globl	runtime_start
	.type	runtime_start,@function
	.align	16

// r9  = suspend flag
// r12 = text address
// r13 = stack limit
// r14 = memory address
// r15 = current memory limit
// mm0 = trap handler
// mm1 = memory growth limit
runtime_start:
	// unmap loader text section
	mov	$GATE_LOADER_TEXT_ADDR, %rdi
	mov	$GATE_LOADER_TEXT_SIZE, %esi
	mov     $SYS_munmap, %eax
	syscall
	mov     $41, %edi
	test    %rax, %rax
	jne     .Lexit

	// enable seccomp (r9 is ignored with PR_SET_SECCOMP)
	mov	$22, %edi		// PR_SET_SECCOMP
	mov	$1, %esi		// SECCOMP_MODE_STRICT
	xor	%edx, %edx
	xor	%r8d, %r8d
	mov	$SYS_prctl, %eax
	syscall
	mov	$43, %edi
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

	.globl	signal_handler
	.type	signal_handler,@function
	.align	16

// edi = signum
signal_handler:
	movq	$1, GATE_SIGNAL_STACK_R9_OFFSET(%rsp)
	ret

	.globl	signal_restorer
	.type	signal_restorer,@function
	.align	16

signal_restorer:
	mov	$SYS_rt_sigreturn, %eax
	syscall
	int3

	.globl	trap_handler
	.type	trap_handler,@function
	.align	16

// eax = trap id
trap_handler:
	test	%eax, %eax
	je	.Ltrap_exit

	cmp	$1, %eax
	je	.Ltrap_missing_function

	mov	%eax, %edi		// zero-extend
	add	$100, %edi		// status (1st syscall arg)
	jmp	.Lexit

.Ltrap_exit:
	mov	$1, %edi		// failure status (1st syscall arg)
	shrq	$32, %rax		// exit code
	cmove	%eax, %edi		// success status (1st syscall arg)
	jmp	.Lexit

.Ltrap_missing_function:
	// using 40 bytes of the 128-byte stack red zone

	// save registers possibly used as parameters of the missing function
	mov	%rcx, -8(%rsp)		// clobbered by syscall
	mov	%rsi, -16(%rsp)		// used for syscall arg
	mov	%rdi, -24(%rsp)		// used for syscall arg
	mov	%r11, -32(%rsp)		// clobbered by syscall

	mov	$GATE_OUTPUT_FD, %edi	// fd (1st syscall arg)
	lea	-40(%rsp), %rsi		// ptr (2nd syscall arg)
	mov	$8, %edx		// size (3rd syscall arg)

	mov	%edx, (%rsi)		// op packet: size
	movl	$255, 4(%rsi)		// op packet: code, flags

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

	.globl	__gate_send
	.type	__gate_send,@function
	.align	128 // important ABI functions fit in 128 bytes; align naturally

// ecx = relative addr
// ebx = size
__gate_send:
	mov	$SYS_write, %ebp
	mov	$GATE_OUTPUT_FD, %edi	// fd (1st syscall arg)
	jmp	.Lio

	.globl	__gate_recv
	.type	__gate_recv,@function
	.align	16

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
	mov	$14, %edi		// status (1st syscall arg)
	cmp	$-EAGAIN, %rax
	jne	.Lexit
	mov	%edx, %eax		// remaining size (result)
	ret

.Lbounds:
	mov	$13, %edi		// status (1st syscall arg)
	jmp	.Lexit

	.globl	__gate_exit
	.type	__gate_exit,@function
	.align	16

// ecx = status number
__gate_exit:
	xor	%edi, %edi
	test	%ecx, %ecx
	setne	%dil			// status boolean (1st syscall arg)
	// fall through to .Lexit

	.globl	runtime_exit
	.type	runtime_exit,@function

// ecx = status code
runtime_exit:
.Lexit:
	sub	$8+128, %r13		// stack reserve
	mov	%rsp, %rax
	sub	%r13, %rax
	mov	%rax, (%r13)		// first word of stack space

	mov	$SYS_exit, %eax
	syscall
	int3

	.globl	__gate_func_ptr
	.type	__gate_func_ptr,@function
	.align	16

__gate_func_ptr:
	xor	%eax, %eax		// null ptr (result)
	ret

	.globl	__gate_get_abi_version
	.type	__gate_get_abi_version,@function
	.align	16

__gate_get_abi_version:
	mov	$GATE_ABI_VERSION, %eax
	ret

	.globl	__gate_get_max_packet_size
	.type	__gate_get_max_packet_size,@function
	.align	16

__gate_get_max_packet_size:
	mov	$GATE_MAX_PACKET_SIZE, %eax
	ret

	.globl	__gate_debug_write
	.type	__gate_debug_write,@function
	.align	16

// ecx = relative addr
// ebx = size
__gate_debug_write:
	mov	$SYS_write, %ebp
	mov	$GATE_DEBUG_FD, %edi	// fd (1st syscall arg)
	jmp	.Lio

// don't want executable stack
.section .note.GNU-stack,"",%progbits
