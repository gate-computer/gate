// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"

	"gate.computer/ga"
	"gate.computer/ga/linux"
	runtimeerrors "gate.computer/gate/internal/error/runtime"
	"gate.computer/gate/internal/executable"
	"gate.computer/gate/trap"
	"gate.computer/wag/object/abi"
	"golang.org/x/sys/unix"
)

const (
	verbose  = false
	assemble = false
)

const (
	AT_SYSINFO_EHDR = 33

	SECCOMP_SET_MODE_FILTER = 1
)

const (
	sizeofStructPollfd    = 8  // sizeof(struct pollfd)
	sizeofStructSockFprog = 16 // sizeof(struct sock_fprog)
	sizeofStructTimespec  = 16 // sizeof(struct timespec)
)

// Register offsets within ucontext_t.
var (
	ucontextStackLimit    = ga.Specific{AMD64: 128, ARM64: 408}
	ucontextStackPtrAMD64 = 160
	ucontextLinkARM64     = 424
	ucontextInsnPtr       = ga.Specific{AMD64: 168, ARM64: 440}
)

var (
	result = ga.Reg{AMD64: ga.RAX, ARM64: ga.X0, Use: "result"}

	// Parameter registers for syscalls, library calls and some macros.
	param0    = ga.Reg{AMD64: ga.RDI, ARM64: ga.X0, Use: "param0"}
	param1    = ga.Reg{AMD64: ga.RSI, ARM64: ga.X1, Use: "param1"}
	param2    = ga.Reg{AMD64: ga.RDX, ARM64: ga.X2, Use: "param2"}
	sysparam3 = ga.Reg{AMD64: ga.R10, ARM64: ga.X3, Use: "sysparam3"}

	// Local registers are preserved across syscalls and library calls,
	// but may be clobbered by some macros.
	local0 = ga.Reg{AMD64: ga.RBP, ARM64: ga.X11, Use: "local0"}
	local1 = ga.Reg{AMD64: ga.R12, ARM64: ga.X12, Use: "local1"}
	local2 = ga.Reg{AMD64: ga.R13, ARM64: ga.X13, Use: "local2"}
	local3 = ga.Reg{AMD64: ga.R14, ARM64: ga.X14, Use: "local3"}

	// Scratch registers may be clobbered by syscalls, library calls and
	// some macros.
	scratch0 = ga.Reg{AMD64: ga.R9, ARM64: ga.X9, Use: "scratch0"}
	scratch1 = ga.Reg{AMD64: ga.R8, ARM64: ga.X8, Use: "scratch1"}

	wagTrap            = ga.Reg{AMD64: ga.RDX, ARM64: ga.X2, Use: "trap"}
	wagRestartSP       = ga.Reg{AMD64: ga.RBP, ARM64: ga.X3, Use: "restart"}
	wagTextBase        = ga.Reg{AMD64: ga.R15, ARM64: ga.X27, Use: "text"}
	wagStackLimit      = ga.Reg{AMD64: ga.RBX, ARM64: ga.X28, Use: "stacklimit"}
	wagStackLimitShift = ga.Specific{AMD64: 0, ARM64: 4}
)

var (
	clearAMD64 = []ga.RegAMD64{
		// RAX contains result.
		ga.RCX,
		ga.RDX,
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
		ga.R14,
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
		ga.X26,
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

func generateRT(arch ga.Arch, sys *ga.System, variant string) string {
	a := ga.NewAssembly(arch, sys)

	funcRTStart(a)
	funcSignalHandler(a)
	funcSignalRestorer(a)
	funcTrapHandler(a)
	funcCurrentMemory(a)
	funcGrowMemory(a, variant)
	funcRTNop(a)
	routineTrampoline(a)
	funcRTPoll(a)
	funcIO(a, "rt_read", linux.SYS_READ, inputFD, ga.GE, runtimeerrors.ERR_RT_READ)
	funcIO(a, "rt_write", linux.SYS_WRITE, outputFD, ga.GT, runtimeerrors.ERR_RT_WRITE)
	routineOutOfBounds(a)
	funcRTTime(a)
	funcRTTimemask(a)
	funcRTRandom(a)
	funcRTTrap(a)
	funcRTDebug(a)
	funcRTRead8(a)
	funcRTWrite8(a)

	return a.String()
}

func reset(a *ga.Assembly, regs ...ga.Reg) {
	common := []ga.Reg{
		wagTextBase,
		wagStackLimit,
	}
	a.Reset(append(common, regs...)...)
}

func funcRTStart(a *ga.Assembly) {
	a.FunctionWithoutPrologue("rt_start")
	reset(a)

	// Unmap loader .text and .rodata sections.
	{
		a.MoveImm(param0, executable.LoaderTextAddr) // munmap addr
		a.MoveImm(param1, 65536)                     // munmap length
		a.Syscall(linux.SYS_MUNMAP)
		a.MoveReg(local0, result)

		a.MoveImm(param0, runtimeerrors.ERR_LOAD_MUNMAP_LOADER)
		a.JumpIfImm(ga.NE, local0, 0, "sys_exit")
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
		a.MoveReg(local0, result)

		a.AddImm(a.StackPtr, a.StackPtr, sizeofStructSockFprog) // Release buffer.

		a.MoveImm(param0, runtimeerrors.ERR_LOAD_SECCOMP)
		a.JumpIfImm(ga.NE, local0, 0, "sys_exit")
	}

	a.Label("rt_start_no_sandbox")

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
	var (
		signum   = param0.As("signum")
		siginfo  = param1.As("siginfo")
		ucontext = param2.As("ucontext")
	)

	a.FunctionWithoutPrologue("signal_handler")
	reset(a, signum, siginfo, ucontext)
	{
		a.JumpIfImm(ga.EQ, signum, int(unix.SIGSEGV), ".signal_segv")

		macroStackVars(a, local0, scratch0)
		a.MoveImm64(local1, uint64(1<<62|1)) // Call and loop suspend bits.

		a.Load4Bytes(local2, local0, 20) // suspend_bits
		a.JumpIfBitSet(local2, 1, ".do_not_modify_suspend_reg")

		a.Load(scratch0, ucontext, a.Specify(ucontextStackLimit))
		a.OrReg(scratch0, local1)
		a.Store(ucontext, a.Specify(ucontextStackLimit), scratch0)

		a.Label(".do_not_modify_suspend_reg")

		a.OrImm(local2, 1<<0)
		a.Store4Bytes(local0, 20, local2) // suspend_bits

		a.Jump(".signal_return")
	}

	a.Label(".signal_segv")
	reset(a, signum, siginfo, ucontext)
	{
		a.Load(local1, ucontext, a.Specify(ucontextInsnPtr))

		if a.Arch == ga.ARM64 {
			a.Store(ucontext, ucontextLinkARM64, local1)
		} else {
			a.Load(scratch0, ucontext, ucontextStackPtrAMD64)
			a.SubtractImm(scratch0, 8)
			a.Store(ucontext, ucontextStackPtrAMD64, scratch0)

			a.Store(scratch0, 0, local1)
		}

		a.Address(scratch0, ".signal_segv_exit")
		a.Store(ucontext, a.Specify(ucontextInsnPtr), scratch0)

		a.Jump(".signal_return")
	}

	a.Label(".signal_segv_exit")
	a.Reset()
	{
		a.MoveImm(param0, statusTrapMemoryAccessOutOfBounds)
		a.Jump(".exit")
	}

	a.Label(".signal_return")
	a.Reset()
	{
		macroClearAllRegs(a)
		a.ReturnWithoutEpilogue()
	}
}

func funcSignalRestorer(a *ga.Assembly) {
	a.FunctionWithoutPrologue("signal_restorer")
	reset(a)
	{
		a.Syscall(linux.SYS_RT_SIGRETURN)
		a.Unreachable()
	}
}

func funcTrapHandler(a *ga.Assembly) {
	a.Function("trap_handler")
	reset(a, wagTrap, result)
	{
		a.JumpIfImm(ga.EQ, wagTrap, int(trap.Exit), ".trap_exit")
		a.JumpIfImm(ga.EQ, wagTrap, int(trap.CallStackExhausted), ".trap_call_stack_exhausted")

		a.AddImm(param0, wagTrap, 100) // Convert trap id to status.
		a.Jump(".exit")
	}

	a.Label(".trap_exit")
	reset(a, wagTrap, result)
	{
		macroStackVars(a, local0, scratch0)
		a.Store(local0, 32, result)   // result[0]
		a.MoveRegFloat(scratch0, 0)   // float result
		a.Store(local0, 40, scratch0) // result[1]

		a.MoveImm(param0, 1) // failure
		a.JumpIfImm(ga.NE, wagTrap, 0, ".exit")
		a.MoveImm(param0, 0) // success
		a.Jump(".exit")
	}

	a.Label(".trap_call_stack_exhausted")
	reset(a)
	{
		a.JumpIfBitSet(wagStackLimit, 0, ".trap_suspended")

		a.MoveImm(param0, statusTrapCallStackExhausted)
		a.Jump(".exit")
	}

	a.Label(".trap_suspended")
	reset(a)
	{
		a.MoveImm(param0, statusTrapSuspended)
		a.Jump(".exit")
	}
}

func funcCurrentMemory(a *ga.Assembly) {
	a.Function("current_memory")
	reset(a)
	{
		macroCurrentMemoryPages(a, result, local0, scratch0)
		a.Jump(".resume")
	}
}

func funcGrowMemory(a *ga.Assembly, variant string) {
	errorCode := runtimeerrors.ERR_RT_MPROTECT
	if variant == "android" {
		errorCode = runtimeerrors.ERR_RT_MREMAP
	}

	var (
		incrementPages = result.As("incrementPages")
		stackVars      = local0.As("stackVars")
		oldPages       = local1.As("oldPages")
		newPages       = local2.As("newPages")
	)

	a.Function("grow_memory")
	reset(a, incrementPages)
	{
		macroCurrentMemoryPages(a, oldPages, stackVars, scratch0)

		a.JumpIfImm(ga.EQ, incrementPages, 0, ".grow_memory_done")

		a.AddReg(newPages, oldPages, incrementPages)

		a.Load(scratch0, wagTextBase, -5*8) // memory growth limit in pages
		a.JumpIfReg(ga.GT, newPages, scratch0, ".out_of_memory")

		if variant == "android" {
			a.Load(param0, wagTextBase, -4*8) // mremap old_address

			a.MoveReg(param1, oldPages)
			a.ShiftImm(ga.Left, param1, 16) // mremap old_size

			a.MoveReg(param2, newPages)
			a.ShiftImm(ga.Left, param2, 16) // mremap new_size

			a.MoveImm(sysparam3, 0) // mremap flags

			a.Syscall(linux.SYS_MREMAP)
			a.Load(scratch0, wagTextBase, -4*8) // memory addr
			a.JumpIfReg(ga.NE, result, scratch0, ".grow_memory_error")
		} else {
			a.MoveReg(param1, incrementPages)
			a.ShiftImm(ga.Left, param1, 16) // mprotect len

			a.Load(param0, wagTextBase, -4*8) // memory addr
			a.MoveReg(scratch0, oldPages)
			a.ShiftImm(ga.Left, scratch0, 16)  // old bytes
			a.AddReg(param0, param0, scratch0) // mprotect addr

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
	reset(a)
	{
		a.MoveImm(param0, errorCode)
		a.Jump(".exit")
	}

	a.Label(".out_of_memory")
	reset(a)
	{
		a.MoveImm(result, -1)
		a.Jump(".resume")
	}
}

func funcRTNop(a *ga.Assembly) {
	a.Function("rt_nop")
	a.Label(".resume_zero")
	reset(a, wagRestartSP)
	{
		a.MoveImm(result, 0)

		a.Label(".resume")
		reset(a, result)
		{
			macroClearRegs(a)
			a.AddImm(scratch0, wagTextBase, abi.TextAddrResume)
			a.FunctionEpilogue()
			a.Jump("trampoline")
		}
	}
}

func funcRTPoll(a *ga.Assembly) {
	var (
		input  = local0.As("input")
		output = local1.As("output")
		fds    = local2.As("fds")
	)

	a.Function("rt_poll")
	reset(a, wagRestartSP)
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
		a.Load(scratch1, wagTextBase, -9*8) // time_mask
		a.AndReg(scratch0, scratch1)
		a.Store(param2, 8, scratch0)

		a.Label(".poll")

		a.Load4Bytes(input, a.StackPtr, 32)
		a.Load4Bytes(output, a.StackPtr, 24)
		a.MoveImm(scratch0, inputFD)
		a.MoveImm(scratch1, outputFD)

		fdsSize := sizeofStructPollfd * 2
		a.SubtractImm(a.StackPtr, fdsSize) // Allocate buffer.
		a.MoveReg(fds, a.StackPtr)
		a.Store4Bytes(fds, 0, scratch0) // fds[0].fd
		a.Store4Bytes(fds, 4, input)    // fds[0].events
		a.Store4Bytes(fds, 8, scratch1) // fds[1].fd
		a.Store4Bytes(fds, 12, output)  // fds[1].events

		a.MoveReg(param0, fds)  // ppoll fds
		a.MoveImm(param1, 2)    // ppoll nfds
		a.MoveImm(sysparam3, 0) // ppoll sigmask
		a.Syscall(linux.SYS_PPOLL)

		a.Load4Bytes(input, fds, 4)               // fds[0].events | (fds[0].revents << 16)
		a.Load4Bytes(output, fds, 12)             // fds[1].events | (fds[1].revents << 16)
		a.AddImm(a.StackPtr, a.StackPtr, fdsSize) // Release buffer.

		a.JumpIfImm(ga.GE, result, 0, ".poll_revents")
		a.JumpIfImm(ga.EQ, result, -int(unix.EAGAIN), ".resume_zero")
		a.JumpIfImm(ga.EQ, result, -int(unix.EINTR), ".resume_zero")

		a.MoveImm(param0, runtimeerrors.ERR_RT_PPOLL)
		a.Jump(".exit")
	}

	a.Label(".poll_revents")
	reset(a, input, output)
	// input  = fds[0].events | (fds[0].revents << 16)
	// output = fds[1].events | (fds[1].revents << 16)
	{
		a.ShiftImm(ga.RightLogical, input, 16)
		a.ShiftImm(ga.RightLogical, output, 16)
		a.AndImm(input, 0xffff)  // fds[0].revents
		a.AndImm(output, 0xffff) // fds[1].revents

		a.MoveImm(scratch0, unix.POLLHUP|unix.POLLRDHUP)
		a.AndReg(scratch0, input)
		a.JumpIfImm(ga.NE, scratch0, 0, ".resume_zero") // Being suspended?

		a.MoveReg(scratch0, input)
		a.AndImm(scratch0, ^unix.POLLIN)
		a.JumpIfImm(ga.NE, scratch0, 0, ".exit")

		a.MoveReg(scratch0, output)
		a.AndImm(scratch0, ^unix.POLLOUT)
		a.JumpIfImm(ga.NE, scratch0, 0, ".exit")

		a.MoveReg(result, input)
		a.OrReg(result, output)
		a.Jump(".resume")
	}
}

func funcIO(a *ga.Assembly, name string, nr ga.Syscall, fd int, expect ga.Cond, error int) {
	a.Function(name)
	reset(a)
	// [StackPtr + 16] = buf offset
	// [StackPtr + 8] = buf size
	{
		macroIOPrologue(a, param2, param1, param0, local1, local0, scratch1, scratch0)
		a.MoveImm(param0, fd)
		a.Syscall(nr)

		a.JumpIfImm(expect, result, 0, ".resume")
		a.JumpIfImm(ga.EQ, result, -int(unix.EAGAIN), ".resume_zero")
		a.JumpIfImm(ga.EQ, result, -int(unix.EINTR), ".resume_zero")

		a.MoveImm(param0, error)
		a.Jump(".exit")
	}
}

func funcRTTime(a *ga.Assembly) {
	a.Function("rt_time")
	reset(a, wagRestartSP)
	// [StackPtr + 8] = clock id
	{
		a.Load4Bytes(param0, a.StackPtr, 8)
		stackVars := macroTime(a, ".rt_time")
		a.Load4Bytes(scratch0, a.StackPtr, 8)
		a.JumpIfImm(ga.NE, scratch0, unix.CLOCK_MONOTONIC_COARSE, ".resume")
		macroTimeFixMonotonic(a, stackVars)
		a.Jump(".resume")
	}
}

func funcRTTimemask(a *ga.Assembly) {
	a.Function("rt_timemask")
	reset(a, wagRestartSP)
	{
		a.Load(result, wagTextBase, -9*8) // time_mask
		a.Jump(".resume")
	}
}

func funcRTRandom(a *ga.Assembly) {
	var (
		stackVars = local0.As("stackVars")
		avail     = local1.As("avail")
	)

	a.Function("rt_random")
	reset(a, wagRestartSP)
	{
		macroStackVars(a, stackVars, scratch0)
		a.Load4Bytes(avail, stackVars, 16)
		a.JumpIfImm(ga.EQ, avail, 0, ".no_random")
		a.SubtractImm(avail, 1)
		a.Store4Bytes(stackVars, 16, avail)
		a.AddReg(local1, avail, wagTextBase)
		a.LoadByte(result, local1, -8*8)
		a.Jump(".resume")
	}

	a.Label(".no_random")
	reset(a)
	{
		a.MoveImm(result, -1)
		a.Jump(".resume")
	}
}

func funcRTTrap(a *ga.Assembly) {
	var (
		status        = param0.As("status")
		monotonicTime = param1.As("monotonicTime")
	)

	a.Function("rt_trap")
	reset(a, wagRestartSP)
	// [StackPtr + 8] = status code

	a.Load4Bytes(status, a.StackPtr, 8)
	a.MoveReg(a.StackPtr, wagRestartSP) // Restart caller on resume.

	a.Label(".exit")
	reset(a, status)
	{
		a.Push(status)

		a.MoveImm(param0, unix.CLOCK_MONOTONIC_COARSE)
		{
			stackVars := macroTime(a, ".rt_trap")
			macroTimeFixMonotonic(a, stackVars)
		}
		a.MoveReg(monotonicTime, result)

		a.Pop(status)

		a.Label(".exit_time")
		reset(a, status, monotonicTime)
		{
			var (
				stackVars = local0.As("stackVars")
			)

			macroStackVars(a, stackVars, scratch0)

			a.MoveReg(local1, a.StackPtr)
			a.SubtractReg(local1, stackVars)    // StackVars is at start of stack buffer.
			a.Store4Bytes(stackVars, 0, local1) // stack_unused

			a.Store(stackVars, 8, monotonicTime) // monotonic_time_snapshot

			a.Label("sys_exit")
			reset(a, status)
			{
				a.Syscall(linux.SYS_EXIT_GROUP)
				a.Unreachable()
			}
		}
	}
}

func funcRTDebug(a *ga.Assembly) {
	var (
		ptr    = local1.As("ptr")
		remain = local2.As("remain")
	)

	a.Function("rt_debug")
	reset(a, wagRestartSP)
	// StackPtr + 16 = buf offset
	// StackPtr + 8 = buf size
	{
		macroIOPrologue(a, remain, ptr, local0, param1, param0, scratch1, scratch0)

		a.Label(".debug_loop")
		a.MoveImm(param0, debugFD)
		a.MoveReg(param1, ptr)
		a.MoveReg(param2, remain)
		a.Syscall(linux.SYS_WRITE)

		a.JumpIfImm(ga.GT, result, 0, ".debugged_some")
		a.JumpIfImm(ga.EQ, result, 0, ".resume")
		a.JumpIfImm(ga.EQ, result, -int(unix.EINTR), ".debug_loop")

		a.MoveImm(param0, runtimeerrors.ERR_RT_DEBUG)
		a.Jump(".exit")
	}

	a.Label(".debugged_some")
	reset(a, ptr, remain, result)
	{
		a.SubtractReg(remain, result)
		a.JumpIfImm(ga.EQ, remain, 0, ".resume_zero")

		a.AddReg(ptr, ptr, result)
		a.Jump(".debug_loop")
	}
}

func funcRTRead8(a *ga.Assembly) {
	a.Function("rt_read8")
	reset(a, wagRestartSP)
	{
		a.SubtractImm(a.StackPtr, 8) // Allocate buffer.

		a.Label(".read8_retry")

		a.MoveImm(param0, inputFD)    // fd
		a.MoveReg(param1, a.StackPtr) // buf
		a.MoveImm(param2, 8)          // count
		a.Syscall(linux.SYS_READ)
		a.MoveReg(local1, result)

		a.JumpIfImm(ga.EQ, local1, -int(unix.EAGAIN), ".read8_retry")
		a.JumpIfImm(ga.EQ, local1, -int(unix.EINTR), ".read8_retry")

		a.Pop(local0) // Release buffer.

		a.MoveImm(param0, runtimeerrors.ERR_RT_READ8)
		a.JumpIfImm(ga.NE, local1, 8, ".exit")

		a.MoveReg(result, local0)
		a.Jump(".resume")
	}
}

func funcRTWrite8(a *ga.Assembly) {
	a.Function("rt_write8")
	reset(a, wagRestartSP)
	// [StackPtr + 8] = data
	{
		a.Label(".write8_retry")

		a.MoveImm(param0, outputFD)     // fd
		a.AddImm(param1, a.StackPtr, 8) // buf
		a.MoveImm(param2, 8)            // count
		a.Syscall(linux.SYS_WRITE)

		a.JumpIfImm(ga.EQ, result, 8, ".resume_zero")
		a.JumpIfImm(ga.EQ, result, -int(unix.EAGAIN), ".write8_retry")
		a.JumpIfImm(ga.EQ, result, -int(unix.EINTR), ".write8_retry")

		a.MoveImm(param0, runtimeerrors.ERR_RT_WRITE8)
		a.Jump(".exit")
	}
}

func routineOutOfBounds(a *ga.Assembly) {
	a.Label(".out_of_bounds")
	reset(a)
	{
		a.MoveImm(param0, statusTrapMemoryAccessOutOfBounds)
		a.Jump(".exit")
	}
}

func routineTrampoline(a *ga.Assembly) {
	a.FunctionWithoutPrologue("trampoline")
	reset(a, scratch0)
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

	a.MoveImm(temp, executable.StackLimitOffset)
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

// macroTime makes a function call, so it may clobber anything.  Afterwards
// timestamp will be in result and stack vars in local0.  The stackVars
// register is returned.
func macroTime(a *ga.Assembly, internalNamePrefix string) ga.Reg {
	// param0 = clock id

	var (
		stackVars  = local0.As("stackVars")
		timeSecs   = local1.As("timeSecs")
		timeNanos  = local2.As("timeNanos")
		saveResult = local3.As("saveResult")
	)

	a.SubtractImm(a.StackPtr, sizeofStructTimespec) // Allocate buffer.
	a.MoveReg(param1, a.StackPtr)                   // clock_gettime tp

	macroStackVars(a, stackVars, scratch0)

	if a.Arch == ga.AMD64 {
		ga.AMD64.OrMem4BytesImm(a, stackVars.AMD64, 20, 1<<1) // suspend_bits; don't modify suspend reg.

		a.Push(wagStackLimit)
		a.Push(wagTextBase)
	}

	a.Load(scratch0, wagTextBase, -11*8) // clock_gettime library function
	a.Call("trampoline")
	a.Set(result)

	a.MoveReg(saveResult, result)

	if a.Arch == ga.AMD64 {
		a.Pop(wagTextBase)
		a.Pop(wagStackLimit)

		a.MoveImm(scratch1, 0)
		ga.AMD64.ExchangeMem4BytesReg(a, stackVars.AMD64, 20, scratch1.AMD64) // suspend_bits
		a.JumpIfBitNotSet(scratch1, 0, internalNamePrefix+"_not_suspended")

		a.MoveImm64(scratch0, 0x4000000000000001) // Suspend calls and loops.
		a.OrReg(wagStackLimit, scratch0)
	}

	a.Label(internalNamePrefix + "_not_suspended")

	// Release buffer:
	if sizeofStructTimespec != 8+8 {
		panic("struct timespec size mismatch")
	}
	a.Pop(timeSecs)  // tv_sec
	a.Pop(timeNanos) // tv_nsec

	a.MoveImm(param0, runtimeerrors.ERR_RT_CLOCK_GETTIME)
	a.MoveImm(param1, -1) // Outrageous timestamp.
	a.JumpIfImm(ga.NE, saveResult, 0, ".exit_time")

	a.Load(scratch0, wagTextBase, -9*8) // time_mask
	a.AndReg(timeNanos, scratch0)       // Imprecise tv_nsec.

	// Convert tv_sec to nanoseconds in two steps to avoid unnecessary
	// wrap-around due to signed multiplication.
	a.MultiplyImm(local3, timeSecs, 500000000, scratch0) // 1000000000/(1<<1)
	a.ShiftImm(ga.Left, local3, 1)
	a.AddReg(result, local3, timeNanos) // Total nanoseconds.

	return stackVars
}

// macroTimeFixMonotonic expects the timestamp in result register, and
// stackVars in the specified (local) regiser.
func macroTimeFixMonotonic(a *ga.Assembly, stackVars ga.Reg) {
	a.Load(scratch0, wagTextBase, -10*8) // local_monotonic_time_base
	a.SubtractReg(result, scratch0)
	a.Load(scratch0, stackVars, 8) // monotonic_time_snapshot
	a.AddReg(result, result, scratch0)
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
	a.MoveReg(local0, result)

	a.MoveImm(param0, runtimeerrors.ERR_RT_DEBUG8)
	a.JumpIfImm(ga.NE, local0, 8, "sys_exit")

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

	reset(a)
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

	a.Reset()
}

func maskOut(n uint32) int {
	return int(int32(^n))
}

func generateStart(arch ga.Arch, sys *ga.System) string {
	a := ga.NewAssembly(arch, sys)

	var (
		status = param0.As("status")
		vdso   = param1.As("vdso")
		iter   = param2.As("iter")
	)

	a.FunctionWithoutPrologue("_start")
	a.Reset()
	{
		a.MoveReg(iter, a.StackPtr)

		a.MoveImm(status, runtimeerrors.ERR_LOAD_ARG_ENV)

		a.Load(scratch0, iter, 0) // argc
		a.JumpIfImm(ga.NE, scratch0, 1, ".exit")
		a.AddImm(iter, iter, 8+8+8) // Skip argc, argv[0] and null terminator.

		a.Load(scratch0, iter, 0) // envp
		a.JumpIfImm(ga.NE, scratch0, 0, ".exit")
		a.AddImm(iter, iter, 8)

		a.MoveImm(status, runtimeerrors.ERR_LOAD_NO_VDSO)

		a.Label(".vdso_loop")
		a.Load(scratch0, iter, 0) // Type of auxv entry.
		a.AddImm(iter, iter, 8)
		a.JumpIfImm(ga.EQ, scratch0, AT_SYSINFO_EHDR, ".vdso_found")
		a.JumpIfImm(ga.EQ, scratch0, 0, ".exit")
		a.AddImm(iter, iter, 8) // Skip value.
		a.Jump(".vdso_loop")

		a.Label(".vdso_found")
		a.Load(vdso, iter, 0)
		a.AddImm(iter, iter, 8)

		// iter should be within the highest stack page (determined
		// experimentally using runtime/loader/inspect/stack.cpp).

		a.MoveImm(param0, 0)    // argc
		a.MoveReg(param1, vdso) // argv
		a.MoveReg(param2, iter) // envp
		a.Call("main")
		a.Reset(result)

		a.MoveReg(status, result)

		a.Label(".exit")
		a.Reset(status)
		{
			a.Syscall(linux.SYS_EXIT_GROUP)
			a.Unreachable()
		}
	}

	return a.String()
}

func main() {
	var (
		filename = os.Args[1]
		archname = os.Args[2]
		variant  = os.Args[3]
	)

	arch := ga.Archs[archname]

	sys := ga.Linux()
	sys.StackPtr.ARM64 = ga.X29
	sys.SysParams[0].Use = "param0"
	sys.SysParams[1].Use = "param1"
	sys.SysParams[2].Use = "param2"
	sys.SysResult.Use = "result"
	sys.LibParams[0].Use = "param0"
	sys.LibParams[1].Use = "param1"
	sys.LibParams[2].Use = "param2"
	sys.LibResult.Use = "result"

	var output string
	switch path.Base(filename) {
	case "rt.gen.S":
		output = generateRT(arch, sys, variant)
	case "start.gen.S":
		output = generateStart(arch, sys)
	default:
		fmt.Fprintln(os.Stderr, filename)
		os.Exit(2)
	}

	if verbose {
		fmt.Printf("// %s source:\n%s\n", arch.Machine(), output)
	}
	if assemble {
		as(arch, output)
	}

	if err := ioutil.WriteFile(filename, []byte(output), 0666); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

const boilerplate = `
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
