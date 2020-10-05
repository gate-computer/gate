// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"crypto"
)

const (
	ModuleRefSource = "sha384"
	ModuleRefHash   = crypto.SHA384
)

func (x *ModuleRefs) Len() int           { return len(x.Modules) }
func (x *ModuleRefs) Swap(i, j int)      { x.Modules[i], x.Modules[j] = x.Modules[j], x.Modules[i] }
func (x *ModuleRefs) Less(i, j int) bool { return x.Modules[i].Id < x.Modules[j].Id }

const (
	StateRunning    = State_RUNNING
	StateSuspended  = State_SUSPENDED
	StateHalted     = State_HALTED
	StateTerminated = State_TERMINATED
	StateKilled     = State_KILLED
)

const (
	CauseNormal                        = Cause_NORMAL
	CauseUnreachable                   = Cause_UNREACHABLE
	CauseCallStackExhausted            = Cause_CALL_STACK_EXHAUSTED
	CauseMemoryAccessOutOfBounds       = Cause_MEMORY_ACCESS_OUT_OF_BOUNDS
	CauseIndirectCallIndexOutOfBounds  = Cause_INDIRECT_CALL_INDEX_OUT_OF_BOUNDS
	CauseIndirectCallSignatureMismatch = Cause_INDIRECT_CALL_SIGNATURE_MISMATCH
	CauseIntegerDivideByZero           = Cause_INTEGER_DIVIDE_BY_ZERO
	CauseIntegerOverflow               = Cause_INTEGER_OVERFLOW
	CauseBreakpoint                    = Cause_BREAKPOINT
	CauseABIDeficiency                 = Cause_ABI_DEFICIENCY
	CauseABIViolation                  = Cause_ABI_VIOLATION
	CauseInternal                      = Cause_INTERNAL
)

func (s *Status) Clone() *Status {
	if s == nil {
		return nil
	}
	return &Status{
		State:  s.State,
		Cause:  s.Cause,
		Result: s.Result,
		Error:  s.Error,
	}
}

func (x *Instances) Len() int           { return len(x.Instances) }
func (x *Instances) Swap(i, j int)      { x.Instances[i], x.Instances[j] = x.Instances[j], x.Instances[i] }
func (x *Instances) Less(i, j int) bool { return x.Instances[i].Instance < x.Instances[j].Instance }

const (
	DebugOpConfigGet        = DebugOp_CONFIG_GET
	DebugOpConfigSet        = DebugOp_CONFIG_SET
	DebugOpConfigUnion      = DebugOp_CONFIG_UNION
	DebugOpConfigComplement = DebugOp_CONFIG_COMPLEMENT
	DebugOpReadGlobals      = DebugOp_READ_GLOBALS
	DebugOpReadMemory       = DebugOp_READ_MEMORY
	DebugOpReadStack        = DebugOp_READ_STACK
)
