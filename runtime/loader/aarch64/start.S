// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define __ASSEMBLY__

#include <bits/auxv.h>
#include <sys/syscall.h>

#include "errors.h"

.section .text


.globl	_start
.type	_start,@function

_start:
	mov	x2, sp			// Stack iterator.

	ldr	x1, [x2]		// argc
	cmp	x1, #0
	b.ne	.Lvector_not_empty
	add	x2, x2, #8

	add	x2, x2, #8		// Skip argv null terminator.

	ldr	x1, [x2]
	cmp	x1, #0
	b.ne	.Lvector_not_empty

.Lvdso_loop:
	add	x2, x2, #8		// Skip previous item (envp null terminator or auxv value).

	ldr	x1, [x2]		// Type of auxv entry.
	add	x2, x2, #8
	cmp	x1, #AT_SYSINFO_EHDR
	b.eq	.Lvdso_found
	cmp	x1, #0
	b.ne	.Lvdso_loop

	mov	x0, #ERR_LOAD_NO_VDSO	// Exit status.
	b	.Lexit

.Lvdso_found:
	ldr	x1, [x2]		// vDSO ELF address as argv pointer.
	add	x2, x2, #8

	// Stack iterator is passed to main as envp pointer.
	// At this point it should be well within the highest stack page
	// (determined experimentally using runtime/loader/test/stack.c).

	bl	main
					// Exit status is already in x0.
	b	.Lexit

.Lvector_not_empty:
	mov	x0, #ERR_LOAD_ARG_ENV	// Exit status.
.Lexit:
	mov	x8, #SYS_exit_group
	svc	#0
	brk	#0

// Don't want executable stack.
.section .note.GNU-stack,"",%progbits
