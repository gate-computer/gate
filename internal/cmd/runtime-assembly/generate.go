// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"sort"

	"gate.computer/ga"
	"gate.computer/ga/linux"
	runtimeerrors "gate.computer/gate/internal/error/runtime"
	"gate.computer/gate/trap"
	"gate.computer/wag/object/abi"
	"golang.org/x/sys/unix"
)

const (
	verbose  = false
	assemble = false
)

const (
	SECCOMP_SET_MODE_FILTER = 1
)

const (
	sizeofStructPollfd    = 4 + 4 // sizeof(struct pollfd)
	sizeofStructSockFprog = 16    // sizeof(struct sock_fprog)
)

// Register offsets within ucontext_t.
var (
	ucontextStackLimit    = ga.Specific{AMD64: 128, ARM64: 408}
	ucontextStackPtrAMD64 = 160
	ucontextLinkARM64     = 424
	ucontextInsnPtr       = ga.Specific{AMD64: 168, ARM64: 440}
)

var (
	result = ga.Reg{AMD64: ga.RAX, ARM64: ga.X0}

	// Parameter registers for syscalls, library calls and some macros.
	param0    = ga.Reg{AMD64: ga.RDI, ARM64: ga.X0}
	param1    = ga.Reg{AMD64: ga.RSI, ARM64: ga.X1}
	param2    = ga.Reg{AMD64: ga.RDX, ARM64: ga.X2}
	sysparam3 = ga.Reg{AMD64: ga.R10, ARM64: ga.X3}

	// Local registers are preserved across syscalls and library calls,
	// but may be clobbered by some macros.
	local0 = ga.Reg{AMD64: ga.RBP, ARM64: ga.X11}
	local1 = ga.Reg{AMD64: ga.R12, ARM64: ga.X12}
	local2 = ga.Reg{AMD64: ga.R13, ARM64: ga.X13}
	local3 = ga.Reg{AMD64: ga.R14, ARM64: ga.X14}

	// Scratch registers may be clobbered by syscalls, library calls and
	// some macros.
	scratch0 = ga.Reg{AMD64: ga.R9, ARM64: ga.X9}
	scratch1 = ga.Reg{AMD64: ga.R8, ARM64: ga.X8}

	wagTrap            = ga.Reg{AMD64: ga.RDX, ARM64: ga.X2}
	wagRestartSP       = ga.Reg{AMD64: ga.RBP, ARM64: ga.X3}
	wagTextBase        = ga.Reg{AMD64: ga.R15, ARM64: ga.X27}
	wagStackLimit      = ga.Reg{AMD64: ga.RBX, ARM64: ga.X28}
	wagStackLimitShift = ga.Specific{AMD64: 0, ARM64: 4}
)

var (
	clearAMD64 = []ga.RegAMD64{
		// RAX contains result.
		ga.RCX,
		// RDX is cleared by wag upon resume.
		// RBX is StackLimit.
		// RSP is StackPtr.
		ga.RBP,
		ga.RSI,
		ga.RDI,
		ga.R8,
		// R9 is used for resume address (cleared by retpoline).
		ga.R10,
		ga.R11,
		ga.R12,
		ga.R13,
		// R14 (MemoryBase) is reset by wag upon resume.
		// R15 is TextBase.
	}
	clearARM64 = []ga.RegARM64{
		// X0 contains result.
		ga.X1,
		ga.X2,
		ga.X3,
		ga.X4,
		ga.X5,
		ga.X6,
		ga.X7,
		ga.X8,
		// X9 is used for resume address.
		ga.X10,
		ga.X11,
		ga.X12,
		ga.X13,
		ga.X14,
		ga.X15,
		ga.X16,
		ga.X17,
		ga.X18,
		ga.X19,
		ga.X20,
		ga.X21,
		ga.X22,
		ga.X23,
		ga.X24,
		ga.X25,
		// X26 (MemoryBase) is reset by wag upon resume.
		// X27 is TextBase.
		// X28 is StackLimit.
		// X29 is FakeSP.
		// X30 is LR.
		// X31 is RealSP/ZR.
	}
)

const (
	inputFD  = 0
	outputFD = 1
	debugFD  = 2
)

const (
	statusTrapSuspended               = 100 + int(trap.Suspended)
	statusTrapCallStackExhausted      = 100 + int(trap.CallStackExhausted)
	statusTrapMemoryAccessOutOfBounds = 100 + int(trap.MemoryAccessOutOfBounds)
)

func generate(arch ga.Arch, sys *ga.System, variant string) string {
	a := ga.NewAssembly(arch, sys)

	funcRuntimeInit(a)
	funcSignalHandler(a)
	funcSignalRestorer(a)
	funcTrapHandler(a)
	funcCurrentMemory(a)
	funcGrowMemory(a, variant)
	funcRtNop(a)
	routineTrampoline(a)
	funcRtPoll(a)
	funcIO(a, "rt_read", linux.SYS_READ, inputFD, ga.GE, runtimeerrors.ERR_RT_READ)
	funcIO(a, "rt_write", linux.SYS_WRITE, outputFD, ga.GT, runtimeerrors.ERR_RT_WRITE)
	routineOutOfBounds(a)
	funcRtTime(a)
	funcRtRandom(a)
	funcRtTrap(a)
	funcRtDebug(a)
	funcRtRead8(a)
	funcRtWrite8(a)

	asm := a.String()

	if verbose {
		fmt.Printf("// %s source:\n%s\n", arch.Machine(), asm)
	}
	if assemble {
		as(arch, asm)
	}

	return asm
}

func funcRuntimeInit(a *ga.Assembly) {
	a.FunctionWithoutPrologue("runtime_init")

	// Unmap loader .text and .rodata sections.
	{
		a.MoveDef(param0, "GATE_LOADER_ADDR")
		a.MoveImm(param1, 65536) // munmap length
		a.Syscall(linux.SYS_MUNMAP)

		a.MoveImm(param0, runtimeerrors.ERR_LOAD_MUNMAP_LOADER)
		a.JumpIfImm(ga.NE, result, 0, "sys_exit")
	}

	// Build sock_fprog structure on stack and enable seccomp.
	{
		a.MoveDef(scratch0, ".seccomp_filter_len")
		a.Address(scratch1, ".seccomp_filter")

		a.SubtractImm(a.StackPtr, sizeofStructSockFprog) // Allocate buffer.
		a.MoveReg(param2, a.StackPtr)                    // seccomp args
		a.Store(param2, 0, scratch0)                     // sock_fprog len (also writing over padding)
		a.Store(param2, 8, scratch1)                     // sock_fprog filter

		a.MoveImm(param0, SECCOMP_SET_MODE_FILTER) // seccomp operation
		a.MoveImm(param1, 0)                       // seccomp flags
		a.Syscall(linux.SYS_SECCOMP)

		a.AddImm(a.StackPtr, a.StackPtr, sizeofStructSockFprog) // Release buffer.

		a.MoveImm(param0, runtimeerrors.ERR_LOAD_SECCOMP)
		a.JumpIfImm(ga.NE, result, 0, "sys_exit")
	}

	a.Label("runtime_init_no_sandbox")

	// Terminate in uninitialized state if already suspended.
	{
		a.MoveImm(param0, statusTrapSuspended)
		a.JumpIfBitSet(wagStackLimit, 0, "sys_exit")
	}

	// Mark stack as dirty just before execution.  (If SIGXCPU signal was
	// received just after the above check, the process has about a second
	// worth of CPU time to reach the first suspend check and execute
	// .exit to avoid inconsistent state.)
	{
		macroStackVars(a, local0, scratch0)
		a.MoveImm(local1, -1)            // Sentinel value.
		a.Store4Bytes(local0, 0, local1) // stack_unused
	}

	// Clear registers used by wag codegen.
	{
		a.MoveImm(result, 0)
		macroClearRegs(a)
	}

	// Execute wag object ABI init routine.
	{
		a.MoveReg(scratch0, wagTextBase)     // Init routine address.
		a.AndImm(wagTextBase, maskOut(0xff)) // Actual text base.
		a.Jump("trampoline")
	}
}

func funcSignalHandler(a *ga.Assembly) {
	a.FunctionWithoutPrologue("signal_handler")
	// param0 = signum
	// param1 = siginfo
	// param2 = ucontext
	{
		a.JumpIfImm(ga.EQ, param0, int(unix.SIGSEGV), ".signal_segv")

		macroStackVars(a, local0, scratch0)
		a.MoveImm64(local1, uint64(1<<62|1)) // Call and loop suspend bits.

		a.Load4Bytes(local2, local0, 20) // suspend_bits
		a.JumpIfBitSet(local2, 1, ".do_not_modify_suspend_reg")

		a.Load(scratch0, param2, a.Specify(ucontextStackLimit))
		a.OrReg(scratch0, local1)
		a.Store(param2, a.Specify(ucontextStackLimit), scratch0)

		a.Label(".do_not_modify_suspend_reg")

		a.OrImm(local2, 1<<0)
		a.Store4Bytes(local0, 20, local2) // suspend_bits

		a.Jump(".signal_return")
	}

	a.Label(".signal_segv")
	// param0 = signum
	// param1 = siginfo
	// param2 = ucontext
	{
		a.Load(local1, param2, a.Specify(ucontextInsnPtr))

		if a.Arch == ga.ARM64 {
			a.Store(param2, ucontextLinkARM64, local1)
		} else {
			a.Load(scratch0, param2, ucontextStackPtrAMD64)
			a.SubtractImm(scratch0, 8)
			a.Store(param2, ucontextStackPtrAMD64, scratch0)

			a.Store(scratch0, 0, local1)
		}

		a.Address(scratch0, ".signal_segv_exit")
		a.Store(param2, a.Specify(ucontextInsnPtr), scratch0)

		a.Jump(".signal_return")
	}

	a.Label(".signal_segv_exit")
	{
		a.MoveImm(param0, statusTrapMemoryAccessOutOfBounds)
		a.Jump(".exit")
	}

	a.Label(".signal_return")
	{
		macroClearAllRegs(a)
		a.ReturnWithoutEpilogue()
	}
}

func funcSignalRestorer(a *ga.Assembly) {
	a.FunctionWithoutPrologue("signal_restorer")
	{
		a.Syscall(linux.SYS_RT_SIGRETURN)
		a.Unreachable()
	}
}

func funcTrapHandler(a *ga.Assembly) {
	a.Function("trap_handler")
	// wagTrap = trap id
	{
		a.MoveReg(param0, wagTrap)

		a.JumpIfImm(ga.EQ, param0, int(trap.Exit), ".trap_exit")
		a.JumpIfImm(ga.EQ, param0, int(trap.CallStackExhausted), ".trap_call_stack_exhausted")

		a.AddImm(param0, param0, 100) // Convert trap id to status.
		a.Jump(".exit")
	}

	a.Label(".trap_exit")
	// param0 = trap id
	{
		macroStackVars(a, local0, scratch0)
		a.Store(local0, 32, result) // result[0]
		a.MoveRegFloat(scratch0, 0)
		a.Store(local0, 40, scratch0) // result[1]

		a.JumpIfImm(ga.NE, param0, 0, ".exit_failure")
		a.Jump(".exit")
	}

	a.Label(".trap_call_stack_exhausted")
	{
		a.JumpIfBitSet(wagStackLimit, 0, ".trap_suspended")

		a.MoveImm(param0, statusTrapCallStackExhausted)
		a.Jump(".exit")
	}

	a.Label(".trap_suspended")
	{
		a.MoveImm(param0, statusTrapSuspended)
		a.Jump(".exit")
	}
}

func funcCurrentMemory(a *ga.Assembly) {
	a.Function("current_memory")
	{
		macroCurrentMemoryPages(a, result, local0, scratch0)
		a.Jump(".resume")
	}
}

func funcGrowMemory(a *ga.Assembly, variant string) {
	a.Function("grow_memory")
	// result = increment in pages
	{
		var (
			stackVars = local0
			oldPages  = local1
			newPages  = local2
			memAddr   = param0
			oldBytes  ga.Reg
		)
		if variant == "android" {
			oldBytes = param1
		} else {
			oldBytes = local3
		}

		macroCurrentMemoryPages(a, oldPages, stackVars, scratch0)

		a.JumpIfImm(ga.EQ, result, 0, ".grow_memory_done")

		a.AddReg(newPages, oldPages, result)

		a.Load(scratch0, wagTextBase, -5*8) // memory growth limit in pages
		a.JumpIfReg(ga.GT, newPages, scratch0, ".out_of_memory")

		a.Load(memAddr, wagTextBase, -4*8) // mremap old_address

		a.MoveReg(oldBytes, oldPages)
		a.ShiftImm(ga.Left, oldBytes, 16) // mremap old_size

		if variant == "android" {
			a.MoveReg(param2, newPages)
			a.ShiftImm(ga.Left, param2, 16) // mremap new_size

			a.MoveImm(sysparam3, 0) // mremap flags

			a.Syscall(linux.SYS_MREMAP)
			a.JumpIfReg(ga.NE, result, memAddr, ".grow_memory_error")
		} else {
			a.AddReg(memAddr, memAddr, oldBytes) // mprotect addr

			a.MoveReg(param1, result)
			a.ShiftImm(ga.Left, param1, 16) // mprotect len

			a.MoveImm(param2, unix.PROT_READ|unix.PROT_WRITE) // mprotect prot

			a.Syscall(linux.SYS_MPROTECT)
			a.JumpIfImm(ga.NE, result, 0, ".grow_memory_error")
		}

		a.Store4Bytes(stackVars, 4, newPages)

		a.Label(".grow_memory_done")

		a.MoveReg(result, oldPages)
		a.Jump(".resume")
	}

	a.Label(".grow_memory_error")
	{
		a.MoveImm(param0, runtimeerrors.ERR_RT_MPROTECT)
		a.Jump(".exit")
	}

	a.Label(".out_of_memory")
	{
		a.MoveImm(result, -1)
		a.Jump(".resume")
	}
}

func funcRtNop(a *ga.Assembly) {
	a.Function("rt_nop")
	a.Label(".resume_zero")
	{
		a.MoveImm(result, 0)

		a.Label(".resume")
		{
			macroClearRegs(a)
			a.AddImm(scratch0, wagTextBase, abi.TextAddrResume)
			a.FunctionEpilogue()
			a.Jump("trampoline")
		}
	}
}

func funcRtPoll(a *ga.Assembly) {
	a.Function("rt_poll")
	// [StackPtr + 32] = input events
	// [StackPtr + 24] = output events
	// [StackPtr + 16] = timeout nanoseconds
	// [StackPtr + 8] = timeout seconds (negative means no timeout)
	{
		a.AddImm(param2, a.StackPtr, 8) // ppoll tmo_p (stack layout coincides with timespec)
		a.Load(scratch0, param2, 0)     // timeout seconds
		a.JumpIfImm(ga.GE, scratch0, 0, ".poll_with_timeout")
		a.MoveImm(param2, 0) // ppoll tmo_p (no timeout)
		a.Jump(".poll")

		a.Label(".poll_with_timeout")

		a.Load(scratch0, param2, 8)         // timeout nanoseconds
		a.Load(scratch1, wagTextBase, -9*8) // mask
		a.AndReg(scratch0, scratch1)
		a.Store(param2, 8, scratch0)

		a.Label(".poll")

		a.Load4Bytes(local0, a.StackPtr, 32)
		a.Load4Bytes(local1, a.StackPtr, 24)
		a.MoveImm(scratch0, inputFD)
		a.MoveImm(scratch1, outputFD)

		fdsSize := sizeofStructPollfd * 2
		a.SubtractImm(a.StackPtr, fdsSize) // Allocate buffer.
		a.MoveReg(param0, a.StackPtr)      // ppoll fds
		a.Store4Bytes(param0, 0, scratch0) // fds[0].fd
		a.Store4Bytes(param0, 4, local0)   // fds[0].events
		a.Store4Bytes(param0, 8, scratch1) // fds[1].fd
		a.Store4Bytes(param0, 12, local1)  // fds[1].events

		a.MoveImm(param1, 2)    // ppoll nfds
		a.MoveImm(sysparam3, 0) // ppoll sigmask
		a.Syscall(linux.SYS_PPOLL)

		a.Load4Bytes(local0, param0, 4)           // fds[0].events | (fds[0].revents << 16)
		a.Load4Bytes(local1, param0, 12)          // fds[1].events | (fds[1].revents << 16)
		a.AddImm(a.StackPtr, a.StackPtr, fdsSize) // Release buffer.

		a.JumpIfImm(ga.GE, result, 0, ".poll_revents")
		a.JumpIfImm(ga.EQ, result, -int(unix.EAGAIN), ".resume_zero")
		a.JumpIfImm(ga.EQ, result, -int(unix.EINTR), ".resume_zero")

		a.MoveImm(param0, runtimeerrors.ERR_RT_POLL)
		a.Jump(".exit")
	}

	a.Label(".poll_revents")
	// local0 = fds[0].events | (fds[0].revents << 16)
	// local1 = fds[1].events | (fds[1].revents << 16)
	{
		a.ShiftImm(ga.RightLogical, local0, 16)
		a.ShiftImm(ga.RightLogical, local1, 16)
		a.AndImm(local0, 0xffff) // fds[0].revents
		a.AndImm(local1, 0xffff) // fds[1].revents

		a.MoveImm(scratch0, unix.POLLHUP|unix.POLLRDHUP)
		a.AndReg(scratch0, local0)
		a.JumpIfImm(ga.NE, scratch0, 0, ".resume_zero") // Being suspended?

		a.MoveReg(scratch0, local0)
		a.AndImm(scratch0, ^unix.POLLIN)
		a.JumpIfImm(ga.NE, scratch0, 0, ".exit")

		a.MoveReg(scratch0, local1)
		a.AndImm(scratch0, ^unix.POLLOUT)
		a.JumpIfImm(ga.NE, scratch0, 0, ".exit")

		a.MoveReg(result, local0)
		a.OrReg(result, local1)
		a.Jump(".resume")
	}
}

func funcIO(a *ga.Assembly, name string, nr ga.Syscall, fd int, expect ga.Cond, error int) {
	a.Function(name)
	// [StackPtr + 16] = buf offset
	// [StackPtr + 8] = buf size
	{
		macroIOPrologue(a, param2, param1, param0, local1, local0, scratch1, scratch0)
		a.MoveImm(param0, fd)
		a.Syscall(nr)

		a.JumpIfImm(expect, result, 0, ".resume")
		a.JumpIfImm(ga.EQ, result, -int(unix.EAGAIN), ".resume_zero")
		a.JumpIfImm(ga.EQ, result, -int(unix.EAGAIN), ".resume_zero")

		a.MoveImm(param0, error)
		a.Jump(".exit")
	}
}

func funcRtTime(a *ga.Assembly) {
	a.Function("rt_time")
	// [StackPtr + 8] = clock id
	{
		a.Load4Bytes(param0, a.StackPtr, 8)
		macroTime(a, ".rt_time")
		a.Jump(".resume")
	}
}

func funcRtRandom(a *ga.Assembly) {
	a.Function("rt_random")
	{
		macroStackVars(a, local0, scratch0)
		a.Load4Bytes(local1, local0, 16) // random_avail
		a.JumpIfImm(ga.EQ, local1, 0, ".no_random")
		a.SubtractImm(local1, 1)
		a.Store4Bytes(local0, 16, local1) // random_avail
		a.AddReg(local1, local1, wagTextBase)
		a.LoadByte(result, local1, -8*8)
		a.Jump(".resume")
	}

	a.Label(".no_random")
	{
		a.MoveImm(result, -1)
		a.Jump(".resume")
	}
}

func funcRtTrap(a *ga.Assembly) {
	a.Function("rt_trap")
	// [StackPtr + 8] = status code
	{
		a.Load4Bytes(param0, a.StackPtr, 8)
		a.MoveReg(a.StackPtr, wagRestartSP) // Restart caller on resume.

		a.Label(".exit")
		// param0 = status code
		{
			a.Push(param0)
			a.MoveImm(param0, unix.CLOCK_MONOTONIC_COARSE)
			macroTime(a, ".rt_trap")
			a.Pop(param0)
			a.MoveReg(param1, result)

			a.Label(".exit_time")
			// param0 = status code
			// param1 = monotonic time
			{
				macroStackVars(a, local0, scratch0)

				a.MoveReg(local1, a.StackPtr)
				a.SubtractReg(local1, local0) // StackVars is at start of stack buffer.

				a.Store4Bytes(local0, 0, local1) // stack_unused
				a.Store(local0, 8, param1)       // monotonic_time_snapshot

				a.Label("sys_exit")
				// param0 = status code
				{
					a.Syscall(linux.SYS_EXIT_GROUP)
					a.Unreachable()
				}
			}
		}
	}

	a.Label(".exit_failure")
	{
		a.MoveImm(param0, 1)
		a.Jump(".exit")
	}
}

func funcRtDebug(a *ga.Assembly) {
	a.Function("rt_debug")
	// StackPtr + 16 = buf offset
	// StackPtr + 8 = buf size
	{
		macroIOPrologue(a, local2, local1, local0, param1, param0, scratch1, scratch0)

		a.Label(".debug_loop")
		a.MoveImm(param0, debugFD)
		a.MoveReg(param1, local1)
		a.MoveReg(param2, local2)
		a.Syscall(linux.SYS_WRITE)

		a.JumpIfImm(ga.GT, result, 0, ".debugged_some")
		a.JumpIfImm(ga.EQ, result, 0, ".resume")
		a.JumpIfImm(ga.EQ, result, -int(unix.EINTR), ".debug_loop")

		a.MoveImm(param0, runtimeerrors.ERR_RT_DEBUG)
		a.Jump(".exit")
	}

	a.Label(".debugged_some")
	{
		a.SubtractReg(local2, result)
		a.JumpIfImm(ga.EQ, local2, 0, ".resume_zero")

		a.AddReg(local1, local1, result)
		a.Jump(".debug_loop")
	}
}

func funcRtRead8(a *ga.Assembly) {
	a.Function("rt_read8")
	{
		a.SubtractImm(a.StackPtr, 8)

		a.Label(".read8_retry")
		a.MoveImm(param0, inputFD)    // fd
		a.MoveReg(param1, a.StackPtr) // buf
		a.MoveImm(param2, 8)          // count
		a.Syscall(linux.SYS_READ)

		a.Pop(local0)

		a.JumpIfImm(ga.EQ, result, -int(unix.EAGAIN), ".read8_retry")

		a.MoveImm(param0, runtimeerrors.ERR_RT_READ)
		a.JumpIfImm(ga.NE, result, 8, ".exit")

		a.MoveReg(result, local0)
		a.Jump(".resume")
	}
}

func funcRtWrite8(a *ga.Assembly) {
	a.Function("rt_write8")
	// [StackPtr + 8] = data
	{
		a.MoveImm(param0, outputFD)     // fd
		a.AddImm(param1, a.StackPtr, 8) // buf
		a.MoveImm(param2, 8)            // count
		a.Syscall(linux.SYS_WRITE)

		a.MoveImm(param0, runtimeerrors.ERR_RT_WRITE)
		a.JumpIfImm(ga.NE, result, 8, ".exit")

		a.Jump(".resume_zero")
	}
}

func routineOutOfBounds(a *ga.Assembly) {
	a.Label(".out_of_bounds")
	{
		a.MoveImm(param0, statusTrapMemoryAccessOutOfBounds)
		a.Jump(".exit")
	}
}

func routineTrampoline(a *ga.Assembly) {
	a.FunctionWithoutPrologue("trampoline")
	// scratch0 = target address
	{
		a.JumpRegRoutine(scratch0, ".trampoline")
	}
}

func macroStackVars(a *ga.Assembly, out, temp ga.Reg) {
	a.MoveReg(out, wagStackLimit)

	a.MoveImm64(temp, ^uint64(1<<62|1)) // Mask out call and loop suspend bits.
	a.AndReg(out, temp)

	a.ShiftImm(ga.Left, out, a.Specify(wagStackLimitShift))

	a.MoveDef(temp, "GATE_STACK_LIMIT_OFFSET")
	a.SubtractReg(out, temp)
}

func macroCurrentMemoryPages(a *ga.Assembly, outPages, outStackVars, temp ga.Reg) {
	macroStackVars(a, outStackVars, temp)
	a.Load4Bytes(outPages, outStackVars, 4)
}

func macroIOPrologue(a *ga.Assembly, outBufSize, outBufAddr, outBufEnd, outMemAddr, outMemEnd, outStackVars, temp ga.Reg) {
	// [StackPtr + 16] = buf offset
	// [StackPtr + 8] = buf size

	a.Load4Bytes(outBufSize, a.StackPtr, 8)
	a.JumpIfImm(ga.EQ, outBufSize, 0, ".resume_zero")

	a.Load(outMemAddr, wagTextBase, -4*8)

	a.Load4Bytes(outBufAddr, a.StackPtr, 16)     // buf offset
	a.AddReg(outBufAddr, outMemAddr, outBufAddr) // outMemAddr + buf offset

	a.AddReg(outBufEnd, outBufAddr, outBufSize)

	macroCurrentMemoryPages(a, outMemEnd, outStackVars, temp) // mem pages
	a.ShiftImm(ga.Left, outMemEnd, 16)                        // mem bytes
	a.AddReg(outMemEnd, outMemAddr, outMemEnd)                // outMemAddr + mem bytes

	a.JumpIfReg(ga.GT, outBufEnd, outMemEnd, ".out_of_bounds")
	a.JumpIfReg(ga.LT, outBufEnd, outBufAddr, ".out_of_bounds")
}

// macroTime makes a function call, so it may clobber anything.
func macroTime(a *ga.Assembly, internalNamePrefix string) {
	// param0 = clock id

	a.MoveReg(local0, param0)

	a.SubtractImm(a.StackPtr, 16) // sizeof(struct timespec)
	a.MoveReg(param1, a.StackPtr) // tp

	macroStackVars(a, local3, scratch0)

	if a.Arch == ga.AMD64 {
		ga.AMD64.OrMem4BytesImm(a, local3.AMD64, 20, 1<<1) // suspend_bits; don't modify suspend reg.

		a.Push(wagStackLimit)
		a.Push(wagTextBase)
	}

	a.Load(scratch0, wagTextBase, -11*8) // clock_gettime library function
	a.Call("trampoline")

	if a.Arch == ga.AMD64 {
		a.Pop(wagTextBase)
		a.Pop(wagStackLimit)

		a.MoveImm(scratch1, 0)
		ga.AMD64.ExchangeMem4BytesReg(a, local3.AMD64, 20, scratch1.AMD64) // suspend_bits
		a.JumpIfBitNotSet(scratch1, 0, internalNamePrefix+"_not_suspended")

		a.MoveImm64(scratch0, 0x4000000000000001) // Suspend calls and loops.
		a.OrReg(wagStackLimit, scratch0)
	}

	a.Label(internalNamePrefix + "_not_suspended")

	a.Pop(local1) // tv_sec
	a.Pop(local2) // tv_nsec

	a.MoveImm(param0, runtimeerrors.ERR_RT_CLOCK_GETTIME)
	a.MoveImm(param1, -1) // Outrageous timestamp.
	a.JumpIfImm(ga.NE, result, 0, ".exit_time")

	a.Load(scratch0, wagTextBase, -9*8) // mask
	a.AndReg(local2, scratch0)          // Imprecise tv_nsec.

	// Convert tv_sec to nanoseconds in two steps to avoid unnecessary
	// wrap-around due to signed multiplication.
	a.MultiplyImm(result, local1, 500000000, scratch0) // 1000000000/(1<<1)
	a.ShiftImm(ga.Left, result, 1)
	a.AddReg(result, result, local2) // Total nanoseconds.

	a.JumpIfImm(ga.NE, local0, unix.CLOCK_MONOTONIC_COARSE, internalNamePrefix+"_got_time")

	a.Load(scratch0, wagTextBase, -10*8) // local_monotonic_time_base
	a.SubtractReg(result, scratch0)
	a.Load(scratch0, local3, 8) // monotonic_time_snapshot
	a.AddReg(result, result, scratch0)

	a.Label(internalNamePrefix + "_got_time")
}

func macroDebug8(a *ga.Assembly, r ga.Reg) {
	a.Push(result)
	a.Push(param0)
	a.Push(param1)
	a.Push(param2)

	a.Push(r) // Allocate buffer.

	a.MoveImm(param0, debugFD)    // fd
	a.MoveReg(param1, a.StackPtr) // buf
	a.MoveImm(param2, 8)          // count
	a.Syscall(linux.SYS_WRITE)

	a.MoveImm(param0, runtimeerrors.ERR_RT_DEBUG)
	a.JumpIfImm(ga.NE, result, 8, "sys_exit")

	a.AddImm(a.StackPtr, a.StackPtr, 8) // Release buffer.

	a.Pop(param2)
	a.Pop(param1)
	a.Pop(param0)
	a.Pop(result)
}

// macroClearRegs clobbers most things.
func macroClearRegs(a *ga.Assembly) {
	if a.Arch == ga.AMD64 {
		for _, r := range clearAMD64 {
			ga.AMD64.ClearReg(a, r)
		}
	} else {
		for _, r := range clearARM64 {
			ga.ARM64.ClearReg(a, r)
		}
	}
}

// macroClearAllRegs clobbers most things.
func macroClearAllRegs(a *ga.Assembly) {
	if a.Arch == ga.AMD64 {
		for _, r := range ga.AMD64.ClearableRegs {
			ga.AMD64.ClearReg(a, r)
		}
	} else {
		for _, r := range ga.ARM64.ClearableRegs {
			ga.ARM64.ClearReg(a, r)
		}
	}
}

func maskOut(n uint32) int {
	return int(int32(^n))
}

func main() {
	sys := ga.Linux()
	sys.StackPtr.ARM64 = ga.X29

	var names []string
	for name := range ga.Archs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, archname := range []string{"amd64", "arm64"} {
		for _, variant := range []string{"", "android"} {
			arch := ga.Archs[archname]
			asm := generate(arch, sys, variant)

			filename := "runtime.S"
			if variant != "" {
				filename = fmt.Sprintf("runtime-%s.S", variant)
			}
			filename = path.Join("runtime", "loader", arch.Machine(), filename)

			if err := ioutil.WriteFile(filename, []byte(asm), 0666); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", archname, err)
				os.Exit(1)
			}
		}
	}
}

const boilerplate = `
.equ GATE_LOADER_ADDR, 0x200000000
.equ GATE_STACK_LIMIT_OFFSET, 0x2700

.Lseccomp_filter:
.int 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
.int 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0

.equ .Lseccomp_filter_len, 128
`

func as(arch ga.Arch, asm string) {
	f, err := ioutil.TempFile("", "*.o")
	if err != nil {
		panic(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	cmd := exec.Command(arch.Machine()+"-linux-gnu-as", "-o", f.Name(), "/dev/stdin")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}

	go func() {
		defer stdin.Close()
		stdin.Write([]byte(boilerplate + asm))
	}()

	if err := cmd.Run(); err != nil {
		panic(err)
	}

	cmd = exec.Command(arch.Machine()+"-linux-gnu-objdump", "-D", f.Name())
	cmd.Stderr = os.Stderr

	dump, err := cmd.Output()
	if err != nil {
		panic(err)
	}

	if verbose {
		fmt.Printf("// %s objdump:\n%s\n", arch.Machine(), dump)
	}
}
