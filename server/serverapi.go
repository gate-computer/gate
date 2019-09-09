// Copyright (c) 2019 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"github.com/tsavola/gate/internal/serverapi"
)

type ModuleRef = serverapi.ModuleRef
type ModuleRefs []ModuleRef

func (a ModuleRefs) Len() int           { return len(a) }
func (a ModuleRefs) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ModuleRefs) Less(i, j int) bool { return a[i].Key < a[j].Key }

type State = serverapi.State

const (
	StateRunning    = serverapi.State_RUNNING
	StateSuspended  = serverapi.State_SUSPENDED
	StateTerminated = serverapi.State_TERMINATED
	StateKilled     = serverapi.State_KILLED
)

type Cause = serverapi.Cause

const (
	CauseUnreachable                   = serverapi.Cause_UNREACHABLE
	CauseCallStackExhausted            = serverapi.Cause_CALL_STACK_EXHAUSTED
	CauseMemoryAccessOutOfBounds       = serverapi.Cause_MEMORY_ACCESS_OUT_OF_BOUNDS
	CauseIndirectCallIndexOutOfBounds  = serverapi.Cause_INDIRECT_CALL_INDEX_OUT_OF_BOUNDS
	CauseIndirectCallSignatureMismatch = serverapi.Cause_INDIRECT_CALL_SIGNATURE_MISMATCH
	CauseIntegerDivideByZero           = serverapi.Cause_INTEGER_DIVIDE_BY_ZERO
	CauseIntegerOverflow               = serverapi.Cause_INTEGER_OVERFLOW
	CauseABIViolation                  = serverapi.Cause_ABI_VIOLATION
)

type Status = serverapi.Status
type InstanceStatus = serverapi.InstanceStatus
type Instances []InstanceStatus

func (a Instances) Len() int           { return len(a) }
func (a Instances) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Instances) Less(i, j int) bool { return a[i].Instance < a[j].Instance }

type IOConnection = serverapi.IOConnection
type ConnectionStatus = serverapi.ConnectionStatus
