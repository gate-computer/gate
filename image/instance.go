// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package image

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/tsavola/gate/internal/error/notfound"
	"github.com/tsavola/gate/internal/error/resourcelimit"
	internal "github.com/tsavola/gate/internal/executable"
	"github.com/tsavola/gate/internal/file"
	"github.com/tsavola/wag/object/abi"
	"github.com/tsavola/wag/object/stack"
	"github.com/tsavola/wag/wa"
)

var ErrInvalidState = errors.New("instance state is invalid")

const (
	instMaxOffset = int64(0x180000000) // 0x80000000 * 3
)

type InstanceStorage interface {
	newInstanceFile() (*file.File, error)
}

// Instance is a program state.  It may be undergoing mutation.
type Instance struct {
	file          *file.File
	initRoutine   uint8
	textAddr      uint64
	stackSize     int
	stackUsage    int
	globalsSize   int
	memorySize    int
	maxMemorySize int
	entryIndex    uint32
	entryAddr     uint32
	coherent      bool
}

func NewInstance(storage InstanceStorage, prog *Program, maxStackSize int, entryIndex, entryAddr uint32,
) (inst *Instance, err error) {
	man := prog.Manifest()

	var (
		instStackSize  = alignPageSize(maxStackSize)
		instStackUsage int
		instTextAddr   uint64
	)

	if man.InitRoutine == abi.TextAddrResume {
		if entryAddr != 0 {
			err = notfound.ErrSuspended
			return
		}

		instStackUsage = int(man.StackUsage)
		instTextAddr = man.TextAddr
	}

	if instStackUsage > instStackSize-internal.StackLimitOffset {
		err = resourcelimit.New("call stack size limit exceeded")
		return
	}

	instFile, err := storage.newInstanceFile()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			instFile.Close()
		}
	}()

	var (
		off1    = progGlobalsOffset
		off2    = int64(instStackSize)
		copyLen = alignPageSize32(man.GlobalsSize) + alignPageSize32(man.MemoryDataSize)
	)

	if instStackUsage > 0 {
		stackCopyLen := alignPageSize(instStackUsage)

		off1 -= int64(stackCopyLen)
		off2 -= int64(stackCopyLen)
		copyLen += stackCopyLen
	}

	err = copyFileRange(prog.file.Fd(), &off1, instFile.Fd(), &off2, copyLen)
	if err != nil {
		return
	}

	inst = &Instance{
		file:          instFile,
		initRoutine:   uint8(man.InitRoutine),
		textAddr:      instTextAddr,
		stackSize:     instStackSize,
		stackUsage:    instStackUsage,
		globalsSize:   int(man.GlobalsSize),
		memorySize:    int(man.MemorySize),
		maxMemorySize: int(man.MemorySizeLimit),
		entryIndex:    entryIndex,
		entryAddr:     entryAddr,
		coherent:      true,
	}
	return
}

func (inst *Instance) Close() (err error) {
	err = inst.file.Close()
	inst.file = nil
	return
}

func (inst *Instance) InitRoutine() uint8 { return inst.initRoutine }
func (inst *Instance) TextAddr() uint64   { return inst.textAddr }
func (inst *Instance) StackSize() int     { return inst.stackSize }
func (inst *Instance) StackUsage() int    { return inst.stackUsage }
func (inst *Instance) GlobalsSize() int   { return alignPageSize(inst.globalsSize) }
func (inst *Instance) MemorySize() int    { return inst.memorySize }
func (inst *Instance) MaxMemorySize() int { return inst.maxMemorySize }
func (inst *Instance) EntryAddr() uint32  { return inst.entryAddr }

// BeginMutation is invoked by a mutator when it takes exclusive ownership of
// the instance state.  CheckMutation and Close may be called during the
// mutation.  The returned file handle is valid until the next Instance method
// call.
func (inst *Instance) BeginMutation(textAddr uint64) (file interface{ Fd() uintptr }, err error) {
	if !inst.coherent {
		err = ErrInvalidState
		return
	}

	inst.textAddr = textAddr
	inst.coherent = false
	file = inst.file
	return
}

// CheckMutation returns nil if the instance state is not undergoing mutation
// and the previous mutator (if any) has terminated cleanly.  ErrInvalidState
// is returned if the opposite is true.  Other errors mean that the check
// failed.
func (inst *Instance) CheckMutation() (err error) {
	if inst.coherent {
		return
	}

	b := make([]byte, 8)

	_, err = inst.file.ReadAt(b, 0)
	if err != nil {
		return
	}

	unused, memorySize, ok := checkStack(b, inst.stackSize)
	if !ok {
		err = ErrInvalidState
		return
	}

	if unused == 0 {
		inst.initRoutine = abi.TextAddrEnter
		inst.stackUsage = 0
	} else {
		inst.initRoutine = abi.TextAddrResume
		inst.stackUsage = inst.stackSize - int(unused)
	}

	inst.memorySize = int(memorySize)
	inst.coherent = true
	return
}

func (inst *Instance) Stacktrace(textMap stack.TextMap, funcSigs []wa.FuncType,
) (stacktrace []stack.Frame, err error) {
	b := make([]byte, inst.stackSize)

	_, err = inst.file.ReadAt(b, 0)
	if err != nil {
		return
	}

	unused, _, ok := checkStack(b, len(b))
	if !ok {
		err = ErrInvalidState
		return
	}

	if unused != 0 && int(unused) != len(b) {
		stacktrace, err = stack.Trace(b[unused:], inst.textAddr, textMap, funcSigs)
	}
	return
}

func checkStack(b []byte, stackSize int) (unused, memorySize uint32, ok bool) {
	if len(b) < 8 {
		return
	}

	memoryPages := binary.LittleEndian.Uint32(b[0:])
	memorySize = memoryPages * wa.PageSize
	unused = binary.LittleEndian.Uint32(b[4:])

	switch {
	case memoryPages > math.MaxInt32/wa.PageSize:
		// Impossible memory state.
		return

	case unused == 0:
		// Suspended before execution started.
		ok = true
		return

	case unused == math.MaxUint32:
		// Execution was suspended by force.
		return

	case unused < internal.StackLimitOffset || unused > uint32(stackSize) || unused&7 != 0:
		// Impossible stack state.
		return

	default:
		ok = true
		return
	}
}

var pageMask = int64(internal.PageSize - 1)

func align8(n int64) int64          { return (n + 7) &^ 7 }
func alignPageSize(n int) int       { return int(alignPageOffset(int64(n))) }
func alignPageSize32(n uint32) int  { return int(alignPageOffset(int64(n))) }
func alignPageOffset(n int64) int64 { return (n + pageMask) &^ pageMask }
