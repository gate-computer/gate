#include <sys/syscall.h>

#include "../defs.h"

	.globl	trap_handler
	.type	trap_handler,@function
	.align	16

// eax = trap id
trap_handler:
	cmp	$1, %eax
	je	.Lmissing_function

	mov	%eax, %edi		// zero-extend status (1st syscall arg)
	test	%edi, %edi
	je	.Lexit

	add	$100, %edi

.Lexit:
	mov	$SYS_exit, %eax
	syscall
	int3

.Lmissing_function:
	// using 40 bytes of the 128-byte stack red zone

	// save registers possibly used as parameters of the missing function
	push	%rcx			// clobbered by syscall
	push	%rsi			// used for syscall arg
	push	%rdi			// used for syscall arg
	push	%r11			// clobbered by syscall

	mov	$1, %edi		// stdout fd (1st syscall arg)
	lea	-8(%rsp), %rsi		// ptr (2nd syscall arg)
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
	lea	-1(%rsp), %rsi		// ptr (2nd syscall arg)
	mov	$1, %edx		// size (3rd syscall arg)
	mov	$SYS_read, %eax
	syscall
	test	%rax, %rax
	jl	.Lerror			// EOF is ok

	pop	%r11
	pop	%rdi
	pop	%rsi
	pop	%rcx

	// rewind return address before the call instruction that got us here
	sub	$5, (%rsp)
	ret

	.globl	__gate_send_full
	.type	__gate_send_full,@function
	.align	64 // important ABI functions fit within single cache line

// ecx = relative addr
// ebx = size
__gate_send_full:
	mov	$1, %edi		// SYS_write/stdout fd (1st syscall arg)
	jmp	.Lio

	.globl	__gate_recv_full
	.type	__gate_recv_full,@function
	.align	16

// ecx = relative addr
// ebx = size
__gate_recv_full:
	xor	%edi, %edi		// SYS_read/stdin fd (1st syscall arg)
	// fall through to .Lio

// ecx = relative addr
// ebx = size
// rdi = syscall/fd (1st syscall arg)
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
	mov	%edi, %eax		// syscall
	syscall
	test	%eax, %eax		// 32-bit test ok; size limit is less than 0x80000000
	jle	.Lerror

	add	%rax, %rsi		// update absolute addr (2nd syscall arg)
	sub	%eax, %edx		// update size (3rd syscall arg)
	jne	.Lloop_io
.Lempty:
	ret

.Lbounds:
	mov	$SYS_exit, %eax
	mov	$13, %edi		// status (1st syscall arg)
	syscall
	int3

.Lerror:
	mov	$SYS_exit, %eax
	mov	$14, %edi		// status (1st syscall arg)
	syscall
	int3

	.globl	__gate_exit
	.type	__gate_exit,@function
	.align	16

// ecx = status number
__gate_exit:
	xor	%edi, %edi
	test	%ecx, %ecx
	setne	%dil			// status boolean (1st syscall arg)

	mov	$SYS_exit, %eax
	syscall
	int3

	.globl	__gate_func_ptr
	.type	__gate_func_ptr,@function
	.align	16

__gate_func_ptr:
	xor	%eax, %eax		// result
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

// don't want executable stack
.section .note.GNU-stack,"",%progbits
