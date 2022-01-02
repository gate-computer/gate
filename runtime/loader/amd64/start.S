// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define __ASSEMBLY__

#include <asm/auxvec.h>
#include <sys/syscall.h>

#include "errors.gen.h"

.section .text

.align	16,0
.globl	_start
.type	_start,@function

_start:
	mov	%rsp, %rdx			// Stack iterator

	mov	(%rdx), %rdi			// argc
	dec	%rdi
	jne	.Lvector_not_empty
	add	$24, %rdx			// Skip argc, argv[0] and null terminator

	cmpq	$0, (%rdx)			// env null terminator
	jne	.Lvector_not_empty

.Lvdso_loop:
	add	$8, %rdx			// Skip previous (envp null terminator or auxv value)

	mov	(%rdx), %rbx			// Type of auxv entry
	add	$8, %rdx
	cmp	$AT_SYSINFO_EHDR, %rbx
	je	.Lvdso_found
	test	%rbx, %rbx
	jne	.Lvdso_loop

	mov	$ERR_LOAD_NO_VDSO, %edi
	jmp	.Lexit

.Lvdso_found:
	mov	(%rdx), %rsi			// vDSO ELF address as argv pointer
	add	$8, %rdx

	// Stack iterator is passed to main as envp pointer.
	// At this point it should be well within the highest stack page
	// (determined experimentally using runtime/loader/test/stack.cpp).

	call	main
	mov	%eax, %edi
	jmp	.Lexit

.Lvector_not_empty:
	mov	$ERR_LOAD_ARG_ENV, %edi
.Lexit:
	mov	$SYS_exit_group, %eax
	syscall
	int3

.section .note.GNU-stack,"",%progbits		// Don't want executable stack
