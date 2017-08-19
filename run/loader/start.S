// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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

	// Store stack pointer for unmapping in the enter function.  At this
	// point it should be well within the highest stack page (determined
	// experimentally using tests/stack.c).
	movq	%rsp, %mm7

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
