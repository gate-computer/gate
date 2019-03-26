// Copyright (c) 2017 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define __ASSEMBLY__

#include <bits/auxv.h>
#include <sys/syscall.h>

#include "errors.h"

.section .text

.align	16,0
.globl	_start
.type	_start,@function

_start:
	mov	%rsp, %rax		// Stack iterator.

	mov	(%rax), %rdi		// argc
	test	%rdi, %rdi
	jne	.Lvector_not_empty
	add	$8, %rax

	add	$8, %rax		// Skip argv null terminator.

	mov	%rax, %rdx		// envp
	cmp	$0, (%rdx)
	jne	.Lvector_not_empty

.Lvdso_loop:
	add	$8, %rax		// Skip previous item (envp null terminator or auxv value).

	mov	(%rax), %rbx		// Type of auxv entry.
	add	$8, %rax
	cmp	$AT_SYSINFO_EHDR, %rbx
	je	.Lvdso_found
	test	%rbx, %rbx
	jne	.Lvdso_loop

	mov	$ERR_LOAD_NO_VDSO, %edi	// Exit status.
	jmp	.Lexit

.Lvdso_found:
	mov	(%rax), %rsi		// vDSO ELF address as argv pointer.

	// Store stack iterator for unmapping stack in the enter function.
	// At this point it should be well within the highest stack page
	// (determined experimentally using runtime/loader/test/stack.c).
	movq	%rax, %mm7

	call	main
	mov	%eax, %edi		// Exit status.
	jmp	.Lexit

.Lvector_not_empty:
	mov	$ERR_LOAD_ARG_ENV, %edi	// Exit status.
.Lexit:
	mov	$SYS_exit_group, %eax
	syscall
	int3

// Don't want executable stack.
.section .note.GNU-stack,"",%progbits
