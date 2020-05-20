// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define __ASSEMBLY__

#include <asm/errno.h>
#include <asm/mman.h>
#include <asm/signal.h>
#include <linux/bpf_common.h>

#include "errors.h"
#include "runtime.h"

#include "../poll.S"

#define SYS_close        57
#define SYS_read         63
#define SYS_write        64
#define SYS_ppoll        73
#define SYS_exit_group   94
#define SYS_rt_sigreturn 139
#define SYS_munmap       215
#define SYS_mremap       216
#define SYS_mprotect     226
#define SYS_seccomp      277

#define UCONTEXT_SUSPEND_REG_OFFSET  408 // x28
#define UCONTEXT_LINK_REG_OFFSET     424 // x30
#define UCONTEXT_INSNPTR_REG_OFFSET  440 // pc

// Out: x1 = stack vars
// Clobbers x0.
.macro	stack_vars
	movn	x0, #0x0001, lsl #0		// ffff ffff ffff fffe
	movk	x0, #0xbfff, lsl #48		// bfff ffff ffff fffe
	and	x1, x28, x0			// Clear suspend bits
	mov	x0, #-GATE_STACK_LIMIT_OFFSET
	add	x1, x0, x1, lsl #4		// (x1 << 4) - GATE_STACK_LIMIT_OFFSET
.endm

// Out: x1  = stack vars
//      out = current memory size in wasm pages
// Clobbers x0.
.macro	current_memory_pages out
	stack_vars
	ldr	\out, [x1, #4]
.endm

// Out: x1  = absolute buffer start address
//      x2  = buffer length
//      x11 = absolute buffer end address
//      x13 = current memory end address
//      x26 = memory address
// Clobbers x0.
.macro	io_prologue
	current_memory_pages w13
	ldr	x26, [x27, #-4*8]		// Memory address
	add	x13, x26, x13, lsl #16		// Current memory end address

	ldr	w1, [x29, #8]			// Relative buffer address
	ldr	w2, [x29, #0]			// Buffer length
	cbz	w2, rt_nop			// Nothing to transfer?

	add	x1, x1, x26			// Absolute buffer address
	add	x11, x1, x2			// End of absolute buffer

	cmp	x13, x11			// Check against memory limit
	b.lt	.Lout_of_bounds
	cmp	x11, x1				// ...and detect wrap-around
	b.lt	.Lout_of_bounds
.endm

.section .text

.align	4096
.globl	runtime_code_begin

runtime_code_begin:

	brk	#0

#include "../seccomp.S"

.Lseccomp_filter_len:
	.word	seccomp_filter_len

	brk	#0

.globl	runtime_init
.type	runtime_init,@function
.globl	runtime_init_no_sandbox
.type	runtime_init_no_sandbox,@function

// In:  x27 = text address + init routine
//      x28 = stack limit >> 4
//      x29 = fake stack ptr
//      x30 = link address
runtime_init:

	// Unmap loader .text and .rodata sections

	mov	x0, #GATE_LOADER_ADDR
	mov	x1, #GATE_LOADER_SIZE
	mov	w8, #SYS_munmap
	svc	0
	cmp	w0, #0
	mov	w0, #ERR_LOAD_MUNMAP_LOADER
	b.ne	sys_exit

	// Build sock_fprog structure on stack

	mov	x2, sp				// seccomp args
	adr	x0, .Lseccomp_filter_len	// clang doesn't understand #seccomp_filter_len
	ldr	w0, [x0]			//
	str	x0, [x2, #0]
	adr	x0, .Lseccomp_filter
	str	x0, [x2, #8]

	// Enable seccomp

	mov	w0, #SECCOMP_SET_MODE_FILTER	// seccomp mode
	mov	w1, #0				// seccomp flags
	mov	w8, #SYS_seccomp
	svc	#0

	cmp	w0, #0
	mov	w0, #ERR_LOAD_SECCOMP
	b.ne	sys_exit

	// Fall through to runtime_init_no_sandbox

runtime_init_no_sandbox:

	// Terminate in uninitialized state if already suspended

	mov	w0, #100+2			// Suspended
	tbnz	x28, #0, sys_exit		// Don't use .Lexit

	// Mark stack as dirty just before execution.  (If SIGXCPU signal was
	// received just after the above check, the process has about a second
	// worth of CPU time to reach the first suspend check and execute
	// .Lexit to avoid inconsistent state.)

	stack_vars
	mov	w0, #-1
	str	w0, [x1]			// stack_unused

	// Clear registers used by wag codegen

	mov	x2, #0
	mov	x3, #0
	mov	x4, #0
	mov	x5, #0
	mov	x6, #0
	mov	x7, #0
	mov	x8, #0
	mov	x9, #0
	mov	x10, #0
	mov	x11, #0
	mov	x12, #0
	mov	x13, #0
	mov	x14, #0
	mov	x15, #0
	mov	x16, #0
	mov	x17, #0
	mov	x18, #0
	mov	x19, #0
	mov	x20, #0
	mov	x21, #0
	mov	x22, #0
	mov	x23, #0
	mov	x24, #0
	mov	x25, #0
	mov	x26, #0

	// Execute wag object ABI init routine

	mov	x1, x27				// Init routine address
	mov	x0, #~0x7f
	and	x27, x27, x0			// Text base
	mov	x0, #0				// Result when resuming
	br	x1

.globl	signal_handler
.type	signal_handler,@function

// In:  w0 = signum
//      x1 = siginfo
//      x2 = ucontext
signal_handler:
	cmp	w0, #SIGSEGV
	b.eq	.Lsigsegv

	movz	x0, #0x0001, lsl #0		// 0000 0000 0000 0001
	movk	x0, #0x4000, lsl #48		// 4000 0000 0000 0001
	ldr	x1, [x2, #UCONTEXT_SUSPEND_REG_OFFSET]
	orr	x1, x1, x0			// Suspend calls and loops
	str	x1, [x2, #UCONTEXT_SUSPEND_REG_OFFSET]
	ret

.Lsigsegv:
	ldr	x1, [x2, #UCONTEXT_INSNPTR_REG_OFFSET]
	str	x1, [x2, #UCONTEXT_LINK_REG_OFFSET]

	adr	x1, .Lsigsegv_exit
	str	x1, [x2, #UCONTEXT_INSNPTR_REG_OFFSET]
	ret

.Lsigsegv_exit:
	mov	w9, #100+5			// MemoryAccessOutOfBounds
	b	.Lexit

.globl	signal_restorer
.type	signal_restorer,@function

signal_restorer:
	mov	w8, #SYS_rt_sigreturn
	svc	#0
	brk	#0

.globl	trap_handler
.type	trap_handler,@function

// In:  x0 = (result << 32) | trap_id
trap_handler:
	cbz	w0, .Ltrap_exit
	cmp	w0, #4				// CallStackExhausted
	b.eq	.Ltrap_call_stack_exhausted

	add	w9, w0, #100
	b	.Lexit

.Ltrap_exit:
	cmp	xzr, x0, lsr #32		// Program result
	cset	w9, ne				// Status depending on result
	b	.Lexit

.Ltrap_call_stack_exhausted:
	tbnz	x28, #0, .Ltrap_suspended

	mov	w9, #100+4			// CallStackExhausted
	b	.Lexit

.Ltrap_suspended:
	mov	w9, #100+2			// Suspended
	b	.Lexit

.globl	current_memory
.type	current_memory,@function

// Out: w0 = allocated wasm pages
current_memory:
	current_memory_pages w0
	b	.Lresume

.globl	grow_memory
.type	grow_memory,@function

// In:  w0 = increment in wasm pages
// Out: w0 = previously allocated wasm pages
grow_memory:
	mov	w12, w0

	current_memory_pages w0
	add	x13, x0, x12			// New memory pages
	ldr	x1, [x27, #-5*8]		// Memory grow limit in pages
	cmp	x1, x13
	b.lt	.Loom

	ldr	x26, [x27, #-4*8]		// Memory address

#ifdef __ANDROID__

	mov	x3, #0				// mremap flags
	lsl	x2, x13, #16			// mremap new_size
	lsl	x1, x0, #16			// mremap old_size
	mov	x0, x26				// mremap old_addr

	mov	w8, #SYS_mremap
	svc	#0

	mov	w9, #ERR_RT_MREMAP
	and	w0, w0, #0xfff
	cbnz	w0, .Lexit

#else // !__ANDROID__

	add	x0, x26, x0, lsl #16		// mprotect addr

	cbz	x12, .Lgrow_done
	lsl	x1, x12, #16			// mprotect len

	mov	w2, #PROT_READ|PROT_WRITE
	mov	w8, #SYS_mprotect
	svc	#0

	mov	w9, #ERR_RT_MPROTECT
	cbnz	w0, .Lexit

#endif // __ANDROID__

.Lgrow_done:
	current_memory_pages w0
	str	w13, [x1, #4]			// current_memory_pages
	b	.Lresume

.Loom:
	mov	w0, #-1
	b	.Lresume

.globl	rt_nop
.type	rt_nop,@function

rt_nop:
	mov	x0, #0

	// Fall through to .Lresume.

.Lresume:
	add	x1, x27, #16			// wag object ABI resume routine
	br	x1

.globl	rt_poll
.type	rt_poll,@function

rt_poll:
	ldr	w4, [x29, #8]			// Input events
	ldr	w5, [x29, #0]			// Output events

	sub	x0, x29, #8*2			// sizeof(struct pollfd) * 2
	str	wzr, [x0, #0]			// fds[0].fd <- GATE_INPUT_FD
	str	w4, [x0, #4]			// fds[0].events and .revents
	mov	w6, #GATE_OUTPUT_FD
	str	w6, [x0, #8]			// fds[1].fd
	str	w5, [x0, #12]			// fds[1].events and .revents

	mov	w1, #2				// nfds
	mov	w2, #0				// tmo_p
	mov	w3, #0				// sigmask
	mov	w8, #SYS_ppoll
	svc	#0

	ldrh	w4, [x29, #-10]			// Input revents
	ldrh	w5, [x29, #-2]			// Output revents

	mov	w9, #ERR_RT_POLL

	cmp	x0, #0
	b.gt	.Lpoll_revents

	cmp	x0, #-EAGAIN
	b.eq	rt_nop
	cmp	x0, #-EINTR
	b.eq	rt_nop

	b	.Lexit

.Lpoll_revents:
	mov	w0, #0

	mov	w6, #POLLHUP|POLLRDHUP
	tst	w4, w6				// Being suspended?
	b.ne	rt_nop
	tst	w4, #~POLLIN
	b.ne	.Lexit
	orr	w0, w0, w4

	tst	w5, #~POLLOUT
	b.ne	.Lexit
	orr	w0, w0, w5

	b	.Lresume

.globl	rt_read
.type	rt_read,@function

rt_read:
	io_prologue

	mov	w0, #GATE_INPUT_FD
	mov	w8, #SYS_read
	svc	#0
	cmp	x0, #0
	b.ge	.Lresume			// EOF if being suspended or killed

	cmp	x0, #-EAGAIN
	b.eq	rt_nop
	cmp	x0, #-EINTR
	b.eq	rt_nop

	mov	w9, #ERR_RT_READ
	b	.Lexit

.globl	rt_write
.type	rt_write,@function

rt_write:
	io_prologue

	mov	w0, #GATE_OUTPUT_FD
	mov	w8, #SYS_write
	svc	#0
	cmp	x0, #0
	b.gt	.Lresume

	cmp	x0, #-EAGAIN
	b.eq	rt_nop
	cmp	x0, #-EINTR
	b.eq	rt_nop

	mov	w9, #ERR_RT_WRITE
	b	.Lexit

.Lout_of_bounds:
	mov	w9, #100+5			// MemoryAccessOutOfBounds
	b	.Lexit

.globl	rt_time
.type	rt_time,@function

rt_time:
	ldr	w0, [x29, #0]			// clk_id

	// Fall through

// In:  w0 = clock id
.Ltime:
	mov	w12, w0

	sub	x29, x29, #16			// sizeof(struct timespec)
	mov	x1, x29				// tp

	mov	x20, x30			// Save link register
	ldr	x8, [x27, #-11*8]		// clock_gettime C ABI function
	blr	x8
	mov	x30, x20			// Restore link register

	ldr	x10, [x29], #8			// tv_sec
	ldr	x11, [x29], #8			// tv_nsec

	mov	w9, #ERR_RT_CLOCK_GETTIME
	mov	x12, #-1			// Outrageous timestamp
	cbnz	w0, .Lexit_time			// Don't use .Lexit

	movz	x3, #0xca00, lsl #16
	movk	x3, #0x3b9a, lsl #0		// 1000000000
	mul	x10, x10, x3
	ldr	w2, [x27, #-9*8]		// mask
	and	x11, x11, x2
	add	x0, x10, x11

	cmp	w12, #6				// CLOCK_MONOTONIC_COARSE
	b.ne	.Lresume

	ldr	x2, [x27, #-10*8]		// local_monotonic_time_base
	sub	x3, x0, x2
	stack_vars
	ldr	x2, [x1, #8]			// monotonic_time_snapshot
	add	x0, x3, x2
	b	.Lresume

.globl	rt_random
.type	rt_random,@function

rt_random:
	stack_vars
	ldr	w2, [x1, #16]			// random_avail
	subs	w2, w2, #1
	b.lt	.Lno_random
	str	w2, [x1, #16]			// random_avail

	add	x1, x27, w2, uxtw		// text + offset
	ldurb	w0, [x1, #-8*8]			// text + offset - random
	b	.Lresume

.Lno_random:
	mov	w0, #-1
	b	.Lresume

.globl	rt_stop
.type	rt_stop,@function

rt_stop:
	ldr	w9, [x29, #0]

	// Fall through

// In:  w9 = status code
.Lexit:
	str	x30, [x29, #-8]!		// Push ABI call site
	str	x9, [x29, #-8]!

	mov	w0, #6				// CLOCK_MONOTONIC_COARSE
	bl	.Ltime
	mov	x12, x0

	ldr	x9, [x29], #8

	// Fall through

// In:  w9  = status code
//	x12 = monotonic time
.Lexit_time:
	stack_vars
	sub	x2, x29, x1
	str	w2, [x1, #0]			// stack_unused
	str	x12, [x1, #8]			// monotonic_time_snapshot

	mov	w0, w9

	// Fall through

.globl	sys_exit
.type	sys_exit,@function

// In:  w0 = status code
sys_exit:
	mov	w8, #SYS_exit_group
	svc	#0
	brk	#0

.globl	rt_debug
.type	rt_debug,@function

rt_debug:
	io_prologue

	mov	w8, #SYS_write
	mov	w0, #GATE_DEBUG_FD
	svc	#0
	cmp	x0, #0
	b.gt	.Ldebugged_some
	b.eq	rt_nop				// EOF; being suspended or killed

	cmp	x0, #-EINTR
	b.eq	rt_debug

	mov	w9, #ERR_RT_DEBUG
	b	.Lexit

.Ldebugged_some:
	add	w1, w1, w0
	str	w1, [x29, #8]
	subs	w2, w2, w0
	str	w2, [x29, #0]
	b.eq	rt_nop

	b	rt_debug

.globl	runtime_code_end

runtime_code_end:

.section .note.GNU-stack,"",%progbits		// Don't want executable stack
