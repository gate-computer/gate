// Copyright (c) 2020 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"crypto"
	"encoding/hex"

	"gate.computer/gate/server/api/pb"
)

type Sortable interface {
	Len() int
	Swap(i, j int)
	Less(i, j int) bool
}

const (
	KnownModuleSource = "sha256"
	KnownModuleHash   = crypto.SHA256
)

func EncodeKnownModule(hashSum []byte) string {
	return hex.EncodeToString(hashSum)
}

type ModuleInfo = pb.ModuleInfo
type Modules = pb.Modules

func SortableModules(x *Modules) Sortable {
	return sortableModules{x.Modules}
}

type sortableModules struct {
	a []*ModuleInfo
}

func (x sortableModules) Len() int           { return len(x.a) }
func (x sortableModules) Swap(i, j int)      { x.a[i], x.a[j] = x.a[j], x.a[i] }
func (x sortableModules) Less(i, j int) bool { return x.a[i].Id < x.a[j].Id }

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

type State = pb.State
type Cause = pb.Cause
type Status = pb.Status

func CloneStatus(s *Status) *Status {
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

type InstanceInfo = pb.InstanceInfo
type Instances = pb.Instances

func SortableInstances(x *Instances) Sortable {
	return sortableInstances{x.Instances}
}

type sortableInstances struct {
	a []*InstanceInfo
}

func (x sortableInstances) Len() int           { return len(x.a) }
func (x sortableInstances) Swap(i, j int)      { x.a[i], x.a[j] = x.a[j], x.a[i] }
func (x sortableInstances) Less(i, j int) bool { return x.a[i].Instance < x.a[j].Instance }

const (
	DebugOpConfigGet        = pb.DebugOp_CONFIG_GET
	DebugOpConfigSet        = pb.DebugOp_CONFIG_SET
	DebugOpConfigUnion      = pb.DebugOp_CONFIG_UNION
	DebugOpConfigComplement = pb.DebugOp_CONFIG_COMPLEMENT
	DebugOpReadGlobals      = pb.DebugOp_READ_GLOBALS
	DebugOpReadMemory       = pb.DebugOp_READ_MEMORY
	DebugOpReadStack        = pb.DebugOp_READ_STACK
)
