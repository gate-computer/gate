#include <sys/syscall.h>

	.section .abi,"ax",@progbits	// TODO: remove readability?

	.globl	abi_exit
	.type	abi_exit,@function
	.align	16

abi_exit:
	mov	%edi, %edi		// %rdi = arg #1 (status) with high 32 bits cleared
	xor	%rax, %rax
	inc	%rax
	test	%rdi, %rdi
	cmovnz	%rax, %rdi
_sys_exit:
	mov	$SYS_exit, %rax
	syscall
	hlt

	.globl	abi_recv
	.type	abi_recv,@function
	.align	16

abi_recv:
	xor	%rdi, %rdi		// %rdi = arg #1 (stdin fd)
	mov	%esi, %esi		// %rsi = arg #2 (data) with high 32 bits cleared
	mov	%edx, %edx		// %rdx = arg #3 (size) with high 32 bits cleared
	mov	%esi, %eax
	add	%edx, %eax
	jc	1f
	mov	$SYS_read, %eax
	syscall
	test	%rax, %rax
	js	2f			// TODO: fail on EOF
	ret
1:
	mov	$11, %edi
	jmp	_sys_exit
2:
	mov	$12, %edi
	jmp	_sys_exit

	.globl	abi_send
	.type	abi_send,@function
	.align	16

abi_send:
	xor	%rdi, %rdi
	inc	%rdi			// %rdi = arg #1 (stdout fd)
	mov	%esi, %esi		// %rsi = arg #2 (data) with high 32 bits cleared
	mov	%edx, %edx		// %rdx = arg #3 (size) with high 32 bits cleared
	mov	%esi, %eax
	add	%edx, %eax
	jc	1f
	mov	$SYS_write, %rax
	syscall
	test	%rax, %rax
	js	2f			// TODO: fail on EOF
	ret
1:
	mov	$13, %edi
	jmp	_sys_exit
2:
	mov	$14, %edi
	jmp	_sys_exit
