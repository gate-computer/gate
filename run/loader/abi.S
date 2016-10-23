#include <sys/syscall.h>

	.globl	__gate_send_full
	.type	__gate_send_full,@function
	.align	16

// ecx = relative addr
// ebx = size
__gate_send_full:
	mov	$1, %edi		// stdout fd (1st syscall arg)
	mov	%edi, %ebp		// SYS_write
	jmp	io

	.globl	__gate_recv_full
	.type	__gate_recv_full,@function
	.align	16

// ecx = relative addr
// ebx = size
__gate_recv_full:
	xor	%edi, %edi		// stdout fd (1st syscall arg)
	xor	%ebp, %ebp		// SYS_read
	// fall through to io

// ecx = relative addr
// ebx = size
// rdi = fd (1st syscall arg)
// rbp = syscall
io:
	// check lower bound of relative addr
	and	%ecx, %ecx		// zero-extend relative addr + test
	js	.bounds

	lea	(%r14,%rcx), %rsi	// absolute addr (2nd syscall arg)

	// check lower bound of size
	mov	%ebx, %edx		// zero-extend size (3rd syscall arg)
	test	%edx, %edx
	je	.empty
	js	.bounds

	// check upper bound of absolute addr + size
	lea	(%rsi,%rdx), %r11
	cmp	%r11, %r15		// against current memory limit
	jl	.bounds
	cmp	%rsi, %r11		// against absolute addr (wrap-around)
	jl	.bounds
.loop:
	mov	%ebp, %eax		// syscall
	syscall
	test	%rax, %rax
	jle	.error

	add	%rax, %rsi		// update absolute addr (2nd syscall arg)
	sub	%eax, %edx		// update size (3rd syscall arg)
	jne	.loop
.empty:
	ret

.bounds:
	mov	$SYS_exit, %eax
	mov	$13, %edi		// status (1st syscall arg)
	syscall
	int3

.error:
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

#include "../defs.h"

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
