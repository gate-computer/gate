// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	pb "gate.computer/gate/pb/server"
)

type (
	Cause          = pb.Cause
	DebugConfig    = pb.DebugConfig
	DebugOp        = pb.DebugOp
	DebugRequest   = pb.DebugRequest
	DebugResponse  = pb.DebugResponse
	Features       = pb.Features
	InstanceInfo   = pb.InstanceInfo
	InstanceUpdate = pb.InstanceUpdate
	Instances      = pb.Instances
	InvokeOptions  = pb.InvokeOptions
	LaunchOptions  = pb.LaunchOptions
	ModuleInfo     = pb.ModuleInfo
	ModuleOptions  = pb.ModuleOptions
	Modules        = pb.Modules
	ResumeOptions  = pb.ResumeOptions
	State          = pb.State
	Status         = pb.Status
)

const (
	StateRunning    = pb.State_RUNNING
	StateSuspended  = pb.State_SUSPENDED
	StateHalted     = pb.State_HALTED
	StateTerminated = pb.State_TERMINATED
	StateKilled     = pb.State_KILLED
)

const (
	CauseNormal                        = pb.Cause_NORMAL
	CauseUnreachable                   = pb.Cause_UNREACHABLE
	CauseCallStackExhausted            = pb.Cause_CALL_STACK_EXHAUSTED
	CauseMemoryAccessOutOfBounds       = pb.Cause_MEMORY_ACCESS_OUT_OF_BOUNDS
	CauseIndirectCallIndexOutOfBounds  = pb.Cause_INDIRECT_CALL_INDEX_OUT_OF_BOUNDS
	CauseIndirectCallSignatureMismatch = pb.Cause_INDIRECT_CALL_SIGNATURE_MISMATCH
	CauseIntegerDivideByZero           = pb.Cause_INTEGER_DIVIDE_BY_ZERO
	CauseIntegerOverflow               = pb.Cause_INTEGER_OVERFLOW
	CauseBreakpoint                    = pb.Cause_BREAKPOINT
	CauseABIDeficiency                 = pb.Cause_ABI_DEFICIENCY
	CauseABIViolation                  = pb.Cause_ABI_VIOLATION
	CauseInternal                      = pb.Cause_INTERNAL
)

const (
	DebugOpConfigGet        = pb.DebugOp_CONFIG_GET
	DebugOpConfigSet        = pb.DebugOp_CONFIG_SET
	DebugOpConfigUnion      = pb.DebugOp_CONFIG_UNION
	DebugOpConfigComplement = pb.DebugOp_CONFIG_COMPLEMENT
	DebugOpReadGlobals      = pb.DebugOp_READ_GLOBALS
	DebugOpReadMemory       = pb.DebugOp_READ_MEMORY
	DebugOpReadStack        = pb.DebugOp_READ_STACK
)
