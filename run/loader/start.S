#include <sys/syscall.h>

	.section .text

	.globl	_start
	.type	_start,@function

_start:
	mov	%rsp, %rax		// save stack top for main

	pop	%rdi			// argc
	test	%rdi, %rdi
	jne	.Ltoo_many_args

	mov	%rsp, %rsi		// argv
	add	$8, %rsp		// skip argv null terminator

	mov	%rsp, %rdx		// envp

	mov	%rax, %rsp		// restore stack top for main
	call	main
	mov	%eax, %edi		// exit status
	jmp	.Lexit

.Ltoo_many_args:
	mov	$48, %edi		// exit status
.Lexit:
	mov	$SYS_exit, %eax
	syscall
	int3

	// don't want executable stack
	.section .note.GNU-stack,"",%progbits
