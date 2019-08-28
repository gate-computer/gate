// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define __ASSEMBLY__

#include <asm/errno.h>
#include <asm/mman.h>
#include <asm/signal.h>
#include <linux/bpf_common.h>
#include <sys/syscall.h>

#include "errors.h"
#include "runtime.h"

#include "../msg.S"

#define UCONTEXT_SUSPEND_REG_OFFSET  408 // x28
#define UCONTEXT_STACKPTR_REG_OFFSET 416 // x29
#define UCONTEXT_INSNPTR_REG_OFFSET  440 // pc

// Out: x1 = start of stack buffer
// Clobbers r0.
.macro	stack_buffer
	movn	x0, #0x0001, lsl #0		// ffff ffff ffff fffe
	movk	x0, #0x7fff, lsl #48		// 7fff ffff ffff fffe
	and	x1, x28, x0			// Clear suspend bits.
	mov	x0, #-GATE_STACK_LIMIT_OFFSET
	add	x1, x0, x1, lsl #4		// (x1 << 4) - GATE_STACK_LIMIT_OFFSET
.endm

// Out: x1 = start of stack buffer
//      out = current memory size in wasm pages
// Clobbers r0.
.macro	current_memory out
	stack_buffer
	ldr	\out, [x1]
.endm

// In:  x12 = relative length address (zero-extended 32-bit value)
//      x13 = current memory end address
// Out: x12 = absolute length address
//      x13 = current memory end address
// Clobbers r11.
.macro	io_len_check nop_label
	cbz	w12, \nop_label

	add	x12, x26, x12			// Absolute length address.
	add	x11, x12, #4			// End of absolute length.

	cmp	x13, x11			// Check against memory limit,
	b.lt	.Lout_of_bounds
	cmp	x11, x12			// and detect wrap-around.
	b.lt	.Lout_of_bounds
.endm

// In:  x1 = relative buffer address (zero-extended 32-bit value)
//      x12 = absolute length address
//      x13 = current memory end address
// Out: x1 = absolute buffer address
//      x2 = length
//      x12 = absolute length address
//      x13 = current memory end address
// Clobbers r11.
.macro	io_setup nop_label
	ldr	w2, [x12]			// Length (zero-extended).
	cbz	w2, \nop_label			// Nothing to transfer.

	add	x1, x1, x26			// Absolute buffer address.
	add	x11, x1, x2			// End of absolute buffer.

	cmp	x13, x11			// Check against memory limit,
	b.lt	.Lout_of_bounds			//
	cmp	x11, x1				// and detect wrap-around.
	b.lt	.Lout_of_bounds			//
.endm

.section .text

.globl	runtime_code_begin

runtime_code_begin:

	brk	#0

#include "../seccomp.S"

	brk	#0

.globl	runtime_init
.type	runtime_init,@function
.globl	runtime_init_no_sandbox
.type	runtime_init_no_sandbox,@function

// In:  x26 = memory address
//      x27 = text address + init routine
//      x28 = stack limit >> 4
//      x29 = fake stack ptr
runtime_init:
	// Unmap loader .text and .rodata sections.

	mov	x0, #GATE_LOADER_ADDR
	mov	x1, #GATE_LOADER_SIZE
	mov	w8, #SYS_munmap
	svc	0
	cmp	w0, #0
	mov	w0, #ERR_LOAD_MUNMAP_LOADER
	b.ne	sys_exit

	// Build sock_fprog structure on stack.  Using 16 bytes of red zone.

	mov	x2, sp				// seccomp args
	mov	x0, #(.Lseccomp_filters_end - .Lseccomp_filters) / 8
	str	x0, [x2, #0]
	adr	x0, .Lseccomp_filters
	str	x0, [x2, #8]

	// Enable seccomp.

	mov	w0, #SECCOMP_SET_MODE_FILTER	// seccomp mode
	mov	w1, #0				// seccomp flags
	mov	w8, #SYS_seccomp
	svc	#0

	cmp	w0, #0
	mov	w0, #ERR_LOAD_SECCOMP
	b.ne	sys_exit

	// Fall through to runtime_init_no_sandbox.

runtime_init_no_sandbox:
	// Clear registers used by wag codegen.

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

	// Already suspended?  Terminate in uninitialized state.

	tbz	x28, #0, .Lproceed

	mov	w0, #100+2			// Suspended
	b	sys_exit			// Don't use .Lruntime_exit

.Lproceed:
	// Mark stack as dirty just before execution.  (If SIGXCPU signal was
	// received just after the above check, the process has one second
	// worth of CPU time to reach the first suspend check in generated code
	// or ABI call and execute .Lruntime_exit to avoid inconsistent state.)

	stack_buffer
	mov	w0, #-1
	str	w0, [x1, #4]

	// Execute wag object ABI init routine.

	mov	x1, x27				// Init routine address
	mov	x0, #~0x7f
	and	x27, x27, x0			// Text base
	mov	x0, #0				// Result reg when resuming after ABI call
	br	x1

.globl	signal_handler
.type	signal_handler,@function

// In:  w0 = signum
//      x1 = siginfo
//      x2 = ucontext
signal_handler:
	cmp	w0, #SIGIO
	b.eq	.Lsigio

	cmp	w0, #SIGSEGV
	b.eq	.Lsigsegv

	movz	x0, #0x0001, lsl #0		// 0000 0000 0000 0001
	movk	x0, #0x8000, lsl #48		// 8000 0000 0000 0001
	ldr	x1, [x2, #UCONTEXT_SUSPEND_REG_OFFSET]
	orr	x1, x1, x0			// Suspend calls and loops.
	str	x1, [x2, #UCONTEXT_SUSPEND_REG_OFFSET]

.Lsigio:
	ret

.Lsigsegv:
	ldr	x0, [x2, #UCONTEXT_STACKPTR_REG_OFFSET]
	sub	x0, x0, #8
	str	x0, [x2, #UCONTEXT_STACKPTR_REG_OFFSET]

	ldr	x1, [x2, #UCONTEXT_INSNPTR_REG_OFFSET]
	str	x1, [x0]

	adr	x1, .Lsigsegv_exit
	str	x1, [x2, #UCONTEXT_INSNPTR_REG_OFFSET]
	ret

.Lsigsegv_exit:
	mov	w9, #100+5			// MemoryAccessOutOfBounds
	b	.Lruntime_exit

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

	cmp	w0, #2				// Suspended
	b.eq	.Lsuspended

	cmp	w0, #4				// CallStackExhausted
	b.eq	.Lcall_stack_exhausted

	add	w9, w0, #100			// exit status
	b	.Lruntime_exit

.Ltrap_exit:
	cmp	xzr, x0, lsr #32		// Program result
	cset	w9, ne				// exit status bit
	b	.Lruntime_exit

.Lsuspended:
	mov	w9, #100+2			// exit status: Suspended
	b	.Lruntime_exit

.Lcall_stack_exhausted:
	tbnz	x28, #0, .Lsuspended

	mov	w9, #100+4			// exit status: CallStackExhausted
	b	.Lruntime_exit

.globl	current_memory
.type	current_memory,@function

// Out: w0 = allocated wasm pages
current_memory:
	current_memory w0
	b	.Lresume

.globl	grow_memory
.type	grow_memory,@function

// In:  w0 = increment in wasm pages
// Out: w0 = previously allocated wasm pages
grow_memory:
	mov	w12, w0

	current_memory w0
	add	x13, x0, x12			// New memory pages
	ldr	x1, [x27, #-32]			// Grow memory limit pages
	cmp	x1, x13
	b.lt	.Loom

	add	x0, x26, x0, lsl #16		// mprotect addr

	cbz	x12, .Lgrow_done
	lsl	x1, x12, #16			// mprotect len

	mov	w2, #PROT_READ|PROT_WRITE
	mov	w8, #SYS_mprotect
	svc	#0

	mov	w9, #ERR_RT_MPROTECT
	cbnz	w0, .Lruntime_exit

.Lgrow_done:
	current_memory w0
	str	w13, [x1]
	b	.Lresume

.Loom:
	mov	w0, #-1

	// Fall through to .Lresume.

.Lresume:
	add	x1, x27, #16			// wag object ABI resume routine
	br	x1

.globl	gate_io
.type	gate_io,@function

gate_io:
	current_memory w13
	add	x13, x26, x13, lsl #16		// Current memory end address

	// Setup source buffer.

	ldr	w1, [x29, #16]			// Relative buffer address
	ldr	w12, [x29, #8]			// Relative length address
	io_len_check .Lskip_send
	io_setup .Lskip_send

	// Send.

	mov	w8, #SYS_write
	mov	w0, #GATE_OUTPUT_FD
	svc	#0

	// Handle normal write result.

	tbz	x0, #63, .Lsent

	// Handle write errors.

	cmp	x0, #-EAGAIN
	b.eq	.Lnot_sent
	cmp	x0, #-EINTR
	b.eq	.Lnot_sent

	mov	w9, #ERR_RT_WRITE		// exit status
	b	.Lruntime_exit

.Lnot_sent:
	mov	w0, #0				// Sent 0 bytes

.Lsent:
	str	w0, [x12]			// Sent length

.Lskip_send:
	// Setup target buffer.

	ldr	w1, [x29, #32]			// Relative buffer address
	ldr	w12, [x29, #24]			// Relative length address
	io_len_check gate_nop
	io_setup gate_nop

	// Map gate_io flag to recv flag.

	ldr	w3, [x29, #0]			// I/O flags
	mvn	w3, w3, lsl #MSG_DONTWAIT_BIT	// Invert and reposition I/O bit as MSG bit
	and	w3, w3, #MSG_DONTWAIT		// Discard unknown flags

	// No source address/length.

	mov	x4, #0
	mov	x5, #0

	// Receive.

	mov	w8, #SYS_recvfrom
	mov	w0, #GATE_INPUT_FD
	svc	#0

	// Handle normal read result.

	tbz	x0, #63, .Lreceived
	cbz	x0, .Lreceived_eof

	// Handle read errors.

	cmp	x0, #-EAGAIN
	b.eq	.Lnot_received
	cmp	x0, #-EINTR
	b.eq	.Lnot_received

	mov	w9, #ERR_RT_RECVFROM		// exit status
	b	.Lruntime_exit

.Lnot_received:
	mov	w0, #0				// Received 0 bytes

.Lreceived:
	str	w0, [x12]			// Received length
	b	gate_nop

.Lreceived_eof:
	str	w0, [x12]			// Received length
	b	.Lsuspended

.Lout_of_bounds:
	mov	w0, #5				// MemoryAccessOutOfBounds trap
	b	trap_handler

.globl	gate_randomseed
.type	gate_randomseed,@function

gate_randomseed:
	tbnz	x28, #0, .Lsuspended

	ldr	x0, [x27, #-10*8]
	b	.Lresume

.globl	gate_time
.type	gate_time,@function

gate_time:
	tbnz	x28, #0, .Lsuspended

	ldr	w18, [x29, #8]			// Clock id.
	cmp	wzr, w18, lsr #1		// Accept 0 and 1.
	b.ne	.Lclock_error
	add	w18, w18, #5			// Coarse clock (5 or 6).

	current_memory w13
	add	x13, x26, x13, lsl #16		// Current memory end address.

	ldr	w1, [x29, #0]			// Relative buffer address.
	add	x1, x26, x1			// Absolute length address.
	add	x11, x1, #16			// End of absolute buffer.

	cmp	x13, x11			// Check against memory limit,
	b.lt	.Lout_of_bounds			//
	cmp	x11, x1				// and detect wrap-around.
	b.lt	.Lout_of_bounds			//

	mov	w0, w18				// clock_gettime clk_id
	mov	x19, x1				// Save clock_gettime tp

	mov	x20, x30			// Save link register.

	ldr	x8, [x27, #-6*8]		// clock_gettime C ABI function.
	blr	x8

	mov	x30, x20			// Restore link register.

	mov	w9, #ERR_RT_CLOCK_GETTIME	// Exit status.
	cbnz	w0, .Lruntime_exit

	ldr	w0, [x27, #-7*8]		// Time mask.
	ldr	x1, [x19, #8]			// tv_nsec
	and	x1, x1, x0
	str	x1, [x19, #8]

	mov	w0, #0				// Success.
	b	.Lresume

.Lclock_error:
	mov	w0, #-1				// Error: clock id not supported.
	b	.Lresume

.globl	gate_exit
.type	gate_exit,@function

gate_exit:
	ldr	w0, [x29]			// gate_exit status number

	cmp	w0, #0
	cset	w9, ne				// exit syscall status bit

	// Fall through to .Lruntime_exit.

// In:  w9 = status code
.Lruntime_exit:
	str	x30, [x29, #-8]!		// Push ABI caller address.

	stack_buffer
	sub	x2, x29, x1			// Stack offset.
	str	w2, [x1, #4]			// After current memory size.

	mov	w0, w9				// exit syscall status

	// Fall through to sys_exit.

.globl	sys_exit
.type	sys_exit,@function

// In:  w0 = status code
sys_exit:
	mov	w8, #SYS_exit_group
	svc	#0
	brk	#0

.globl	gate_debug
.type	gate_debug,@function

gate_debug:
	current_memory w13
	add	x13, x26, x13, lsl #16		// Current memory end address

	ldr	w1, [x29, #8]			// Relative buffer address
	add	x12, x29, #0			// Absolute length address
	io_setup gate_nop

	mov	w8, #SYS_write
	mov	w0, #GATE_DEBUG_FD
	svc	#0

	cmp	x0, #-EINTR
	b.eq	gate_debug

	mov	w9, #ERR_RT_DEBUG		// exit status
	tbnz	x0, #63, .Lruntime_exit

	add	w1, w1, w0
	str	w1, [x29, #8]
	subs	w2, w2, w0
	str	w2, [x29, #0]
	b.ne	gate_debug

	// Fall through to gate_nop.

.globl	gate_nop
.type	gate_nop,@function

gate_nop:
	tbnz	x28, #0, .Lsuspended

	mov	x0, #0
	b	.Lresume

.globl	runtime_code_end

runtime_code_end:

// Don't want executable stack.
.section .note.GNU-stack,"",%progbits
